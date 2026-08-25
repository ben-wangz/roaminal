package worker

import (
	"bufio"
	"encoding/json"
	"errors"
	"io"
	"strconv"
	"strings"
)

func frame(header any, payload []byte) ([]byte, error) {
	headerBytes, err := json.Marshal(header)
	if err != nil {
		return nil, err
	}
	if len(headerBytes) > HeaderLimit || len(payload) > PayloadLimit {
		return nil, errors.New("worker frame exceeds limit")
	}
	result := make([]byte, 8+len(headerBytes)+len(payload))
	result[0] = byte(len(headerBytes) >> 24)
	result[1] = byte(len(headerBytes) >> 16)
	result[2] = byte(len(headerBytes) >> 8)
	result[3] = byte(len(headerBytes))
	result[4] = byte(len(payload) >> 24)
	result[5] = byte(len(payload) >> 16)
	result[6] = byte(len(payload) >> 8)
	result[7] = byte(len(payload))
	copy(result[8:], headerBytes)
	copy(result[8+len(headerBytes):], payload)
	return result, nil
}

func readFrame(reader *bufio.Reader) (Frame, error) {
	var prefix [8]byte
	if _, err := io.ReadFull(reader, prefix[:]); err != nil {
		return Frame{}, err
	}
	headerLength := int(prefix[0])<<24 | int(prefix[1])<<16 | int(prefix[2])<<8 | int(prefix[3])
	payloadLength := int(prefix[4])<<24 | int(prefix[5])<<16 | int(prefix[6])<<8 | int(prefix[7])
	if headerLength < 1 || headerLength > HeaderLimit || payloadLength < 0 || payloadLength > PayloadLimit {
		return Frame{}, errors.New("invalid worker frame lengths")
	}
	headerBytes := make([]byte, headerLength)
	if _, err := io.ReadFull(reader, headerBytes); err != nil {
		return Frame{}, err
	}
	payload := make([]byte, payloadLength)
	if _, err := io.ReadFull(reader, payload); err != nil {
		return Frame{}, err
	}
	if !json.Valid(headerBytes) || len(headerBytes) < 2 || headerBytes[0] != '{' || headerBytes[len(headerBytes)-1] != '}' {
		return Frame{}, errors.New("invalid worker JSON header")
	}
	return Frame{Header: json.RawMessage(headerBytes), Payload: payload}, nil
}
func validSequence(value string) bool {
	if value == "" || (len(value) > 1 && value[0] == '0') || strings.Trim(value, "0123456789") != "" {
		return false
	}
	_, err := strconv.ParseUint(value, 10, 64)
	return err == nil
}
