package messages

import (
	"bytes"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"io"
	"strings"
	"unicode"
	"unicode/utf8"
)

type InspectionMode string

const (
	InspectAuto   InspectionMode = "auto"
	InspectJSON   InspectionMode = "json"
	InspectXML    InspectionMode = "xml"
	InspectText   InspectionMode = "text"
	InspectHex    InspectionMode = "hex"
	InspectBinary InspectionMode = "binary"
)

func InspectPayload(payload []byte, mode InspectionMode) (string, InspectionMode, error) {
	if mode == InspectAuto {
		mode = DetectInspectionMode(payload)
	}
	switch mode {
	case InspectJSON:
		var output bytes.Buffer
		if err := json.Indent(&output, payload, "", "  "); err != nil {
			return "", mode, fmt.Errorf("format JSON payload: %w", err)
		}
		return output.String(), mode, nil
	case InspectXML:
		formatted, err := formatXML(payload)
		if err != nil {
			return "", mode, err
		}
		return formatted, mode, nil
	case InspectText:
		if !utf8.Valid(payload) {
			return "", mode, fmt.Errorf("payload is not valid UTF-8 text")
		}
		return string(payload), mode, nil
	case InspectHex:
		return strings.TrimSuffix(hex.Dump(payload), "\n"), mode, nil
	case InspectBinary:
		return base64.StdEncoding.EncodeToString(payload), mode, nil
	default:
		return "", mode, fmt.Errorf("unsupported inspection mode %q", mode)
	}
}

func DetectInspectionMode(payload []byte) InspectionMode {
	if json.Valid(payload) {
		return InspectJSON
	}
	if validXML(payload) {
		return InspectXML
	}
	if utf8.Valid(payload) {
		for _, value := range string(payload) {
			if unicode.IsControl(value) && value != '\n' && value != '\r' && value != '\t' {
				return InspectHex
			}
		}
		return InspectText
	}
	return InspectHex
}

func validXML(payload []byte) bool {
	decoder := xml.NewDecoder(bytes.NewReader(payload))
	hasElement := false
	for {
		token, err := decoder.Token()
		if err == io.EOF {
			return hasElement
		}
		if err != nil {
			return false
		}
		if _, ok := token.(xml.StartElement); ok {
			hasElement = true
		}
	}
}

func formatXML(payload []byte) (string, error) {
	decoder := xml.NewDecoder(bytes.NewReader(payload))
	var output bytes.Buffer
	encoder := xml.NewEncoder(&output)
	encoder.Indent("", "  ")
	for {
		token, err := decoder.Token()
		if err != nil {
			if err == io.EOF {
				break
			}
			return "", fmt.Errorf("format XML payload: %w", err)
		}
		if err := encoder.EncodeToken(token); err != nil {
			return "", fmt.Errorf("format XML payload: %w", err)
		}
	}
	if err := encoder.Flush(); err != nil {
		return "", fmt.Errorf("format XML payload: %w", err)
	}
	return output.String(), nil
}
