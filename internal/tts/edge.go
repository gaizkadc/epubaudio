package tts

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/gorilla/websocket"
)

// EdgeTTS implements the Synthesizer and VoiceProvider interfaces using Microsoft Edge's online TTS service.
type EdgeTTS struct {
	ProxyURL string
}

// NewEdgeTTS creates an EdgeTTS instance.
func NewEdgeTTS(proxyURL string) *EdgeTTS {
	return &EdgeTTS{
		ProxyURL: proxyURL,
	}
}

// Name returns the provider identifier.
func (e *EdgeTTS) Name() string {
	return "edge-tts"
}

// Synthesize synthesizes speech from text and writes an MP3 file to outPath.
func (e *EdgeTTS) Synthesize(ctx context.Context, text string, outPath string, opts Options) error {
	text = strings.TrimSpace(text)
	if text == "" {
		return nil
	}

	voice := opts.Voice
	if voice == "" {
		voice = "es-ES-AlvaroNeural"
	}

	rate := opts.Rate
	if rate == "" {
		rate = "+0%"
	}
	pitch := opts.Pitch
	if pitch == "" {
		pitch = "+0Hz"
	}
	volume := opts.Volume
	if volume == "" {
		volume = "+0%"
	}

	var lastErr error
	maxRetries := 3

	for attempt := 1; attempt <= maxRetries; attempt++ {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		audioData, err := e.synthesizeWebSocket(ctx, text, voice, rate, pitch, volume)
		if err != nil {
			lastErr = fmt.Errorf("edge-tts stream error (attempt %d/%d): %w", attempt, maxRetries, err)
			time.Sleep(time.Duration(attempt) * 750 * time.Millisecond)
			continue
		}

		if len(audioData) == 0 {
			lastErr = fmt.Errorf("no audio bytes received (attempt %d/%d)", attempt, maxRetries)
			time.Sleep(time.Duration(attempt) * 500 * time.Millisecond)
			continue
		}

		if err := os.MkdirAll(filepath.Dir(outPath), 0755); err != nil {
			return err
		}

		if err := os.WriteFile(outPath, audioData, 0644); err != nil {
			return fmt.Errorf("failed to save audio file %s: %w", outPath, err)
		}

		return nil
	}

	return lastErr
}

func (e *EdgeTTS) synthesizeWebSocket(ctx context.Context, text, voice, rate, pitch, volume string) ([]byte, error) {
	token := GenerateSecMSGecV2()
	connID := strings.ToLower(randomHex(16))
	muid := randomHex(16)

	wsURL := fmt.Sprintf(
		"wss://speech.platform.bing.com/consumer/speech/synthesize/readaloud/edge/v1?TrustedClientToken=%s&Sec-MS-GEC=%s&Sec-MS-GEC-Version=%s&ConnectionId=%s",
		trustedClientToken, token, secMsGecVersion, connID,
	)

	headers := http.Header{}
	headers.Set("User-Agent", fmt.Sprintf("Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/%s.0.0.0 Safari/537.36 Edg/%s.0.0.0", strings.Split(chromiumFullVersion, ".")[0], strings.Split(chromiumFullVersion, ".")[0]))
	headers.Set("Accept-Encoding", "gzip, deflate, br, zstd")
	headers.Set("Accept-Language", "en-US,en;q=0.9")
	headers.Set("Pragma", "no-cache")
	headers.Set("Cache-Control", "no-cache")
	headers.Set("Origin", "chrome-extension://jdiccldimpdaibmpdkjnbmckianbfold")
	headers.Set("Cookie", fmt.Sprintf("muid=%s;", muid))

	dialer := websocket.Dialer{
		HandshakeTimeout: 15 * time.Second,
	}

	if e.ProxyURL != "" {
		pURL, err := url.Parse(e.ProxyURL)
		if err != nil {
			return nil, fmt.Errorf("invalid proxy URL: %w", err)
		}
		dialer.Proxy = http.ProxyURL(pURL)
	}

	conn, _, err := dialer.DialContext(ctx, wsURL, headers)
	if err != nil {
		return nil, fmt.Errorf("websocket dial failed: %w", err)
	}
	defer conn.Close()

	nowStr := time.Now().UTC().Format(time.RFC1123)

	// 1. Send speech.config
	configMsg := fmt.Sprintf(
		"X-Timestamp:%s\r\nContent-Type:application/json; charset=utf-8\r\nPath:speech.config\r\n\r\n{\"context\":{\"synthesis\":{\"audio\":{\"metadataoptions\":{\"sentenceBoundaryEnabled\":\"false\",\"wordBoundaryEnabled\":\"false\"},\"outputFormat\":\"audio-24khz-48kbitrate-mono-mp3\"}}}}",
		nowStr,
	)
	if err := conn.WriteMessage(websocket.TextMessage, []byte(configMsg)); err != nil {
		return nil, fmt.Errorf("failed to send speech config: %w", err)
	}

	// 2. Send SSML
	reqID := strings.ToLower(randomHex(16))
	ssml := fmt.Sprintf(
		"<speak version='1.0' xmlns='http://www.w3.org/2001/10/synthesis' xml:lang='en-US'><voice name='%s'><prosody pitch='%s' rate='%s' volume='%s'>%s</prosody></voice></speak>",
		xmlEscape(voice), xmlEscape(pitch), xmlEscape(rate), xmlEscape(volume), xmlEscape(text),
	)
	ssmlMsg := fmt.Sprintf(
		"X-RequestId:%s\r\nX-Timestamp:%s\r\nContent-Type:application/ssml+xml\r\nPath:ssml\r\n\r\n%s",
		reqID, nowStr, ssml,
	)
	if err := conn.WriteMessage(websocket.TextMessage, []byte(ssmlMsg)); err != nil {
		return nil, fmt.Errorf("failed to send SSML request: %w", err)
	}

	// 3. Receive audio frames
	var audioBuf bytes.Buffer

	for {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}

		msgType, data, err := conn.ReadMessage()
		if err != nil {
			return nil, err
		}

		if msgType == websocket.TextMessage {
			textStr := string(data)
			if strings.Contains(textStr, "Path:turn.end") {
				break
			}
		} else if msgType == websocket.BinaryMessage {
			if len(data) >= 2 {
				headerLen := int(binary.BigEndian.Uint16(data[:2]))
				if len(data) > 2+headerLen {
					audioBuf.Write(data[2+headerLen:])
				}
			}
		}
	}

	return audioBuf.Bytes(), nil
}

// ListVoices retrieves available voices from Edge TTS.
func (e *EdgeTTS) ListVoices(ctx context.Context) ([]Voice, error) {
	token := GenerateSecMSGecV2()
	apiURL := fmt.Sprintf(
		"https://speech.platform.bing.com/consumer/speech/synthesize/readaloud/voices/list?trustedclienttoken=%s&Sec-MS-GEC=%s&Sec-MS-GEC-Version=%s",
		trustedClientToken, token, secMsGecVersion,
	)

	req, err := http.NewRequestWithContext(ctx, "GET", apiURL, nil)
	if err != nil {
		return nil, err
	}

	req.Header.Set("User-Agent", fmt.Sprintf("Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/%s.0.0.0 Safari/537.36 Edg/%s.0.0.0", strings.Split(chromiumFullVersion, ".")[0], strings.Split(chromiumFullVersion, ".")[0]))
	req.Header.Set("Authority", "speech.platform.bing.com")
	req.Header.Set("Accept", "*/*")

	client := &http.Client{Timeout: 15 * time.Second}
	if e.ProxyURL != "" {
		pURL, err := url.Parse(e.ProxyURL)
		if err == nil {
			client.Transport = &http.Transport{Proxy: http.ProxyURL(pURL)}
		}
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch voice list: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("voice list error (%s): %s", resp.Status, string(body))
	}

	type edgeVoiceResp struct {
		Name         string `json:"Name"`
		ShortName    string `json:"ShortName"`
		Gender       string `json:"Gender"`
		Locale       string `json:"Locale"`
		FriendlyName string `json:"FriendlyName"`
		Status       string `json:"Status"`
	}

	var rawList []edgeVoiceResp
	if err := json.NewDecoder(resp.Body).Decode(&rawList); err != nil {
		return nil, fmt.Errorf("failed to parse voices JSON: %w", err)
	}

	voices := make([]Voice, 0, len(rawList))
	for _, v := range rawList {
		voices = append(voices, Voice{
			ID:          v.ShortName,
			Name:        v.FriendlyName,
			Language:    v.Locale,
			Gender:      v.Gender,
			Description: v.Status,
		})
	}
	return voices, nil
}

func xmlEscape(s string) string {
	var buf strings.Builder
	for _, r := range s {
		switch r {
		case '&':
			buf.WriteString("&amp;")
		case '<':
			buf.WriteString("&lt;")
		case '>':
			buf.WriteString("&gt;")
		case '"':
			buf.WriteString("&quot;")
		case '\'':
			buf.WriteString("&apos;")
		default:
			buf.WriteRune(r)
		}
	}
	return buf.String()
}
