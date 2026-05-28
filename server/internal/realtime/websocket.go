package realtime

import (
	"bufio"
	"crypto/sha1"
	"encoding/base64"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
)

const websocketGUID = "258EAFA5-E914-47DA-95CA-C5AB0DC85B11"

func ServeWebSocket(w http.ResponseWriter, r *http.Request, client *Client, unregister func()) error {
	defer unregister()
	if !strings.EqualFold(r.Header.Get("Upgrade"), "websocket") {
		http.Error(w, "websocket upgrade required", http.StatusUpgradeRequired)
		return errors.New("websocket upgrade required")
	}
	key := r.Header.Get("Sec-WebSocket-Key")
	if key == "" {
		http.Error(w, "missing websocket key", http.StatusBadRequest)
		return errors.New("missing websocket key")
	}
	hijacker, ok := w.(http.Hijacker)
	if !ok {
		http.Error(w, "hijacking unsupported", http.StatusInternalServerError)
		return errors.New("hijacking unsupported")
	}
	conn, rw, err := hijacker.Hijack()
	if err != nil {
		return err
	}
	defer conn.Close()

	accept := websocketAccept(key)
	if _, err := fmt.Fprintf(rw, "HTTP/1.1 101 Switching Protocols\r\nUpgrade: websocket\r\nConnection: Upgrade\r\nSec-WebSocket-Accept: %s\r\n\r\n", accept); err != nil {
		return err
	}
	if err := rw.Flush(); err != nil {
		return err
	}

	done := make(chan struct{})
	go drainClientFrames(conn, done)
	for {
		select {
		case payload, ok := <-client.Send():
			if !ok {
				return nil
			}
			if err := writeTextFrame(rw, payload); err != nil {
				return err
			}
			if err := rw.Flush(); err != nil {
				return err
			}
		case <-done:
			return nil
		case <-r.Context().Done():
			return r.Context().Err()
		}
	}
}

func websocketAccept(key string) string {
	sum := sha1.Sum([]byte(key + websocketGUID))
	return base64.StdEncoding.EncodeToString(sum[:])
}

func writeTextFrame(w io.Writer, payload []byte) error {
	header := []byte{0x81}
	switch {
	case len(payload) <= 125:
		header = append(header, byte(len(payload)))
	case len(payload) <= 65535:
		header = append(header, 126, 0, 0)
		binary.BigEndian.PutUint16(header[2:], uint16(len(payload)))
	default:
		header = append(header, 127, 0, 0, 0, 0, 0, 0, 0, 0)
		binary.BigEndian.PutUint64(header[2:], uint64(len(payload)))
	}
	if _, err := w.Write(header); err != nil {
		return err
	}
	_, err := w.Write(payload)
	return err
}

func drainClientFrames(conn net.Conn, done chan<- struct{}) {
	defer close(done)
	reader := bufio.NewReader(conn)
	for {
		first, err := reader.ReadByte()
		if err != nil {
			return
		}
		second, err := reader.ReadByte()
		if err != nil {
			return
		}
		opcode := first & 0x0f
		masked := second&0x80 != 0
		length := int64(second & 0x7f)
		switch length {
		case 126:
			var buf [2]byte
			if _, err := io.ReadFull(reader, buf[:]); err != nil {
				return
			}
			length = int64(binary.BigEndian.Uint16(buf[:]))
		case 127:
			var buf [8]byte
			if _, err := io.ReadFull(reader, buf[:]); err != nil {
				return
			}
			length = int64(binary.BigEndian.Uint64(buf[:]))
		}
		if masked {
			var mask [4]byte
			if _, err := io.ReadFull(reader, mask[:]); err != nil {
				return
			}
		}
		if length > 0 {
			if _, err := io.CopyN(io.Discard, reader, length); err != nil {
				return
			}
		}
		if opcode == 0x8 {
			return
		}
	}
}
