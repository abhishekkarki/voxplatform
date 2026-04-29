package gateway

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"mime/multipart"
	"net/http"
	"sync"
	"time"

	"nhooyr.io/websocket"
)

// StreamConfig holds configuration for the streaming endpoint.
type StreamConfig struct {
	// MaxBufferDuration is the maximum audio to accumulate before
	// force-flushing to Whisper, even without a silence detection.
	// Prevents memory issues from someone speaking non-stop.
	MaxBufferDuration time.Duration

	// SilenceFlushDelay is how long to wait after VAD reports silence
	// before flushing the buffer. Short pauses between words shouldn't
	// trigger a flush — only real sentence boundaries.
	SilenceFlushDelay time.Duration

	// SampleRate is the expected audio sample rate (16kHz for Whisper).
	SampleRate int

	// FrameSize is bytes per audio frame (20ms of 16kHz 16-bit mono = 640 bytes).
	FrameSize int

	// VADEndpoint is the URL of the VAD sidecar service.
	VADEndpoint string
}

// DefaultStreamConfig returns sensible defaults for streaming.
func DefaultStreamConfig() StreamConfig {
	return StreamConfig{
		MaxBufferDuration: 30 * time.Second,
		SilenceFlushDelay: 800 * time.Millisecond,
		SampleRate:        16000,
		FrameSize:         640, // 20ms at 16kHz, 16-bit mono
		VADEndpoint:       "http://localhost:8001",
	}
}

// StreamMessage is sent back to the client over WebSocket.
type StreamMessage struct {
	Type      string  `json:"type"`                 // "partial", "final", "error"
	Text      string  `json:"text"`                 // Transcribed text
	IsFinal   bool    `json:"is_final"`             // True when utterance is complete
	Duration  float64 `json:"duration_seconds"`     // Processing time
	RequestID string  `json:"request_id,omitempty"` // For debugging
}

// audioBuffer accumulates voiced audio frames and flushes them to Whisper.
// It's the core state machine for streaming:
//
//	IDLE → (speech detected) → BUFFERING → (silence detected) → FLUSHING → IDLE
type audioBuffer struct {
	mu          sync.Mutex
	frames      [][]byte
	totalBytes  int
	speechStart time.Time
	lastSpeech  time.Time
	isSpeaking  bool
}

func newAudioBuffer() *audioBuffer {
	return &audioBuffer{
		frames: make([][]byte, 0, 256),
	}
}

// addFrame appends an audio frame to the buffer.
func (b *audioBuffer) addFrame(frame []byte) {
	b.mu.Lock()
	defer b.mu.Unlock()

	b.frames = append(b.frames, frame)
	b.totalBytes += len(frame)
}

// markSpeech records that speech was detected.
func (b *audioBuffer) markSpeech() {
	b.mu.Lock()
	defer b.mu.Unlock()

	now := time.Now()
	if !b.isSpeaking {
		b.speechStart = now
		b.isSpeaking = true
	}
	b.lastSpeech = now
}

// markSilence records that silence was detected.
func (b *audioBuffer) markSilence() {
	b.mu.Lock()
	defer b.mu.Unlock()

	b.isSpeaking = false
}

// shouldFlush returns true if the buffer should be flushed to Whisper.
// Two conditions: silence after speech (pause detected), or buffer too large.
func (b *audioBuffer) shouldFlush(silenceDelay time.Duration, maxDuration time.Duration, sampleRate int) bool {
	b.mu.Lock()
	defer b.mu.Unlock()

	if len(b.frames) == 0 {
		return false
	}

	// Force flush if buffer is too large (someone speaking non-stop)
	bufferDuration := time.Duration(b.totalBytes/2/sampleRate) * time.Second
	if bufferDuration >= maxDuration {
		return true
	}

	// Flush if we had speech and now silence for long enough
	if !b.isSpeaking && !b.lastSpeech.IsZero() {
		return time.Since(b.lastSpeech) >= silenceDelay
	}

	return false
}

// flush returns all buffered audio as a single WAV byte slice and resets.
func (b *audioBuffer) flush(sampleRate int) []byte {
	b.mu.Lock()
	defer b.mu.Unlock()

	if len(b.frames) == 0 {
		return nil
	}

	// Calculate total PCM data size
	totalPCM := 0
	for _, f := range b.frames {
		totalPCM += len(f)
	}

	// Build a WAV file in memory
	wav := buildWAV(b.frames, totalPCM, sampleRate)

	// Reset buffer
	b.frames = make([][]byte, 0, 256)
	b.totalBytes = 0
	b.lastSpeech = time.Time{}
	b.isSpeaking = false

	return wav
}

// buildWAV creates a valid WAV file from raw PCM frames.
// Whisper expects WAV format, so we wrap the raw audio.
func buildWAV(frames [][]byte, totalPCM int, sampleRate int) []byte {
	// WAV header is 44 bytes
	buf := make([]byte, 44+totalPCM)

	// RIFF header
	copy(buf[0:4], "RIFF")
	putLE32(buf[4:8], uint32(36+totalPCM))
	copy(buf[8:12], "WAVE")

	// fmt chunk
	copy(buf[12:16], "fmt ")
	putLE32(buf[16:20], 16) // chunk size
	putLE16(buf[20:22], 1)  // PCM format
	putLE16(buf[22:24], 1)  // mono
	putLE32(buf[24:28], uint32(sampleRate))
	putLE32(buf[28:32], uint32(sampleRate*2)) // byte rate (16-bit mono)
	putLE16(buf[32:34], 2)                    // block align
	putLE16(buf[34:36], 16)                   // bits per sample

	// data chunk
	copy(buf[36:40], "data")
	putLE32(buf[40:44], uint32(totalPCM))

	// Copy PCM data
	offset := 44
	for _, f := range frames {
		copy(buf[offset:], f)
		offset += len(f)
	}

	return buf
}

func putLE16(b []byte, v uint16) {
	b[0] = byte(v)
	b[1] = byte(v >> 8)
}

func putLE32(b []byte, v uint32) {
	b[0] = byte(v)
	b[1] = byte(v >> 8)
	b[2] = byte(v >> 16)
	b[3] = byte(v >> 24)
}

// handleStream is the WebSocket endpoint for real-time audio streaming.
//
// Protocol:
//
//	Client → Server: binary frames (raw 16kHz 16-bit PCM audio, 20ms chunks)
//	Server → Client: JSON messages (StreamMessage with partial/final transcripts)
//	Client → Server: text message "close" to end the session
func (s *Server) handleStream(w http.ResponseWriter, r *http.Request) {
	requestID := RequestIDFromContext(r.Context())
	logger := slog.With("request_id", requestID, "handler", "stream")

	// Accept the WebSocket connection
	conn, err := websocket.Accept(w, r, &websocket.AcceptOptions{
		InsecureSkipVerify: true, // Allow connections from any origin in dev
	})
	if err != nil {
		logger.Error("websocket accept failed", "error", err)
		return
	}
	defer conn.CloseNow()

	logger.Info("streaming session started")

	ctx, cancel := context.WithCancel(r.Context())
	defer cancel()

	buffer := newAudioBuffer()
	streamCfg := DefaultStreamConfig()

	// Override VAD endpoint from server config if set
	if s.cfg.VADEndpoint != "" {
		streamCfg.VADEndpoint = s.cfg.VADEndpoint
	}

	// Start the flush loop in a goroutine — checks periodically
	// if the buffer should be flushed to Whisper
	go s.flushLoop(ctx, conn, buffer, streamCfg, requestID)

	// Read loop — receives audio frames from the client
	for {
		msgType, data, err := conn.Read(ctx)
		if err != nil {
			if websocket.CloseStatus(err) == websocket.StatusNormalClosure {
				logger.Info("client closed connection")
			} else {
				logger.Warn("websocket read error", "error", err)
			}

			// Flush remaining audio before closing
			s.flushBuffer(ctx, conn, buffer, streamCfg, requestID)
			return
		}

		// Text message = control command
		if msgType == websocket.MessageText {
			if string(data) == "close" {
				logger.Info("client requested close")
				s.flushBuffer(ctx, conn, buffer, streamCfg, requestID)
				conn.Close(websocket.StatusNormalClosure, "session ended")
				return
			}
			continue
		}

		// Binary message = audio frame
		if msgType == websocket.MessageBinary {
			// Check with VAD if this frame contains speech
			isSpeech, err := s.checkVAD(ctx, data, streamCfg.VADEndpoint)
			if err != nil {
				// If VAD is down, assume speech (don't drop audio)
				logger.Warn("VAD check failed, assuming speech", "error", err)
				isSpeech = true
			}

			if isSpeech {
				buffer.markSpeech()
				buffer.addFrame(data)
			} else {
				buffer.markSilence()
				// Still add a few silence frames for natural-sounding audio
				if buffer.isSpeaking {
					buffer.addFrame(data)
				}
			}
		}
	}
}

// flushLoop periodically checks if the buffer should be flushed.
func (s *Server) flushLoop(
	ctx context.Context,
	conn *websocket.Conn,
	buffer *audioBuffer,
	cfg StreamConfig,
	requestID string,
) {
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if buffer.shouldFlush(cfg.SilenceFlushDelay, cfg.MaxBufferDuration, cfg.SampleRate) {
				s.flushBuffer(ctx, conn, buffer, cfg, requestID)
			}
		}
	}
}

// flushBuffer sends buffered audio to Whisper and sends the transcript back.
func (s *Server) flushBuffer(
	ctx context.Context,
	conn *websocket.Conn,
	buffer *audioBuffer,
	cfg StreamConfig,
	requestID string,
) {
	logger := slog.With("request_id", requestID)

	wavData := buffer.flush(cfg.SampleRate)
	if wavData == nil {
		return
	}

	start := time.Now()

	// Build multipart form for the Whisper proxy
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	part, err := writer.CreateFormFile("file", "stream.wav")
	if err != nil {
		logger.Error("creating form file", "error", err)
		return
	}
	io.Copy(part, bytes.NewReader(wavData))
	writer.WriteField("model", "Systran/faster-whisper-small.en")
	writer.Close()

	// Send to Whisper via the proxy's HTTP client
	whisperURL := s.cfg.WhisperURL + "/v1/audio/transcriptions"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, whisperURL, &body)
	if err != nil {
		logger.Error("creating whisper request", "error", err)
		return
	}
	req.Header.Set("Content-Type", writer.FormDataContentType())

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		logger.Error("whisper request failed", "error", err)
		sendStreamMessage(ctx, conn, StreamMessage{
			Type:      "error",
			Text:      fmt.Sprintf("transcription failed: %v", err),
			RequestID: requestID,
		})
		return
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)

	if resp.StatusCode != http.StatusOK {
		logger.Error("whisper error", "status", resp.StatusCode, "body", string(respBody))
		return
	}

	var result struct {
		Text string `json:"text"`
	}
	json.Unmarshal(respBody, &result)

	duration := time.Since(start).Seconds()

	logger.Info("stream transcription",
		"text_length", len(result.Text),
		"duration_seconds", duration,
		"audio_bytes", len(wavData),
	)

	// Send transcript back to client
	sendStreamMessage(ctx, conn, StreamMessage{
		Type:      "final",
		Text:      result.Text,
		IsFinal:   true,
		Duration:  duration,
		RequestID: requestID,
	})
}

// checkVAD sends an audio frame to the VAD service and returns true if speech.
func (s *Server) checkVAD(ctx context.Context, frame []byte, vadURL string) (bool, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, vadURL+"/vad", bytes.NewReader(frame))
	if err != nil {
		return false, err
	}
	req.Header.Set("Content-Type", "application/octet-stream")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return false, err
	}
	defer resp.Body.Close()

	var result struct {
		Speech bool `json:"speech"`
	}
	json.NewDecoder(resp.Body).Decode(&result)

	return result.Speech, nil
}

// sendStreamMessage sends a JSON message over the WebSocket.
func sendStreamMessage(ctx context.Context, conn *websocket.Conn, msg StreamMessage) {
	data, err := json.Marshal(msg)
	if err != nil {
		return
	}
	conn.Write(ctx, websocket.MessageText, data)
}
