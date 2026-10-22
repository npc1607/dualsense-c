package ipc

import (
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
)

// MagicCmd identifies a bidirectional RPC control plane frame ("CMD1").
const MagicCmd uint32 = 0x31444D43

// CmdHeaderSize is the 8-byte framing preamble (4-byte magic + 4-byte payload length).
const CmdHeaderSize = 8

// Command represents a JSON-RPC request sent to the daemon.
type Command struct {
	ID     string          `json:"id"`
	Method string          `json:"method"`
	Params json.RawMessage `json:"params,omitempty"`
}

// Response represents a JSON-RPC reply returned from the daemon.
type Response struct {
	ID     string          `json:"id"`
	OK     bool            `json:"ok"`
	Result json.RawMessage `json:"result,omitempty"`
	Error  string          `json:"error,omitempty"`
}

// CommandHandler processes an incoming Command and returns a Response.
type CommandHandler func(cmd Command) Response

// ReadCommand reads one length-prefixed CMD1 command frame from r.
func ReadCommand(r io.Reader) (*Command, error) {
	var hdr [CmdHeaderSize]byte
	if _, err := io.ReadFull(r, hdr[:]); err != nil {
		return nil, err
	}
	magic := binary.LittleEndian.Uint32(hdr[0:4])
	if magic != MagicCmd {
		return nil, fmt.Errorf("vpad/ipc: invalid cmd frame magic 0x%08x", magic)
	}
	length := binary.LittleEndian.Uint32(hdr[4:8])
	if length > 1024*1024 { // 1MB safety limit
		return nil, fmt.Errorf("vpad/ipc: cmd payload too large (%d)", length)
	}

	payload := make([]byte, length)
	if _, err := io.ReadFull(r, payload); err != nil {
		return nil, err
	}

	var cmd Command
	if err := json.Unmarshal(payload, &cmd); err != nil {
		return nil, fmt.Errorf("vpad/ipc: unmarshal command: %w", err)
	}
	return &cmd, nil
}

// WriteCommand encodes and writes a CMD1 request frame to w.
func WriteCommand(w io.Writer, cmd Command) error {
	payload, err := json.Marshal(cmd)
	if err != nil {
		return fmt.Errorf("vpad/ipc: marshal command: %w", err)
	}

	var hdr [CmdHeaderSize]byte
	binary.LittleEndian.PutUint32(hdr[0:4], MagicCmd)
	binary.LittleEndian.PutUint32(hdr[4:8], uint32(len(payload)))

	if _, err := w.Write(hdr[:]); err != nil {
		return err
	}
	_, err = w.Write(payload)
	return err
}

// ReadResponse reads one length-prefixed CMD1 response frame from r.
func ReadResponse(r io.Reader) (*Response, error) {
	var hdr [CmdHeaderSize]byte
	if _, err := io.ReadFull(r, hdr[:]); err != nil {
		return nil, err
	}
	magic := binary.LittleEndian.Uint32(hdr[0:4])
	if magic != MagicCmd {
		return nil, fmt.Errorf("vpad/ipc: invalid response frame magic 0x%08x", magic)
	}
	length := binary.LittleEndian.Uint32(hdr[4:8])
	if length > 1024*1024 {
		return nil, fmt.Errorf("vpad/ipc: response payload too large (%d)", length)
	}

	payload := make([]byte, length)
	if _, err := io.ReadFull(r, payload); err != nil {
		return nil, err
	}

	var resp Response
	if err := json.Unmarshal(payload, &resp); err != nil {
		return nil, fmt.Errorf("vpad/ipc: unmarshal response: %w", err)
	}
	return &resp, nil
}

// WriteResponse encodes and writes a CMD1 response frame to w.
func WriteResponse(w io.Writer, resp Response) error {
	payload, err := json.Marshal(resp)
	if err != nil {
		return fmt.Errorf("vpad/ipc: marshal response: %w", err)
	}

	var hdr [CmdHeaderSize]byte
	binary.LittleEndian.PutUint32(hdr[0:4], MagicCmd)
	binary.LittleEndian.PutUint32(hdr[4:8], uint32(len(payload)))

	if _, err := w.Write(hdr[:]); err != nil {
		return err
	}
	_, err = w.Write(payload)
	return err
}
