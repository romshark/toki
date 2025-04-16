// Package arb provides a parser for .ARB (Application Resource Bundle) files.
// (See https://github.com/google/app-resource-bundle)
package arb

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"maps"
	"slices"
	"strings"
	"time"

	"github.com/romshark/icumsg"
	"golang.org/x/text/language"
)

type MessageType string

const (
	MessageTypeText  MessageType = "text"
	MessageTypeImage MessageType = "image"
	MessageTypeCSS   MessageType = "css"
)

type PlaceholderType string

const (
	PlaceholderString   PlaceholderType = "String"
	PlaceholderInt      PlaceholderType = "int"
	PlaceholderDouble   PlaceholderType = "double"
	PlaceholderNum      PlaceholderType = "num"
	PlaceholderDateTime PlaceholderType = "DateTime"
)

// File is an ARB (Application Resource Bundle) file.
type File struct {
	Locale           language.Tag
	Context          string         // @@context
	LastModified     time.Time      // @@last_modified
	Author           string         // @@author
	Comment          string         // @@comment
	CustomAttributes map[string]any // @@x-... attributes
	Messages         map[string]Message
}

type Message struct {
	ID               string // "greeting"
	ICUMessage       string // "Hello, {name}!"
	ICUMessageTokens []icumsg.Token
	Description      string                 // @greeting.description
	Comment          string                 // @greeting.comment
	Type             MessageType            // @greeting.type (text/image/css)
	Context          string                 // @greeting.context
	Placeholders     map[string]Placeholder // @greeting.placeholders
	CustomAttributes map[string]any         // @greeting.x-... attributes
}

type Placeholder struct {
	Type               PlaceholderType // "String", "int", "double", "DateTime"
	Description        string
	Example            string
	Format             string // For DateTime or numbers.
	IsCustomDateFormat bool   // Optional.
	OptionalParameters map[string]any
}

var (
	ErrInvalidICUMessage     = errors.New("invalid ICU message")
	ErrMissingRequiredLocale = errors.New("missing required @@locale")
	ErrInvalid               = errors.New("invalid")
	ErrMalformedJSON         = errors.New("malformed JSON")
	ErrEmptyICUMsg           = errors.New("empty ICU message")
	ErrUndefinedPlaceholder  = errors.New("undefined placeholder")
)

type Decoder struct {
	tokenizer icumsg.Tokenizer
	buffer    []icumsg.Token
}

func NewDecoder() *Decoder { return new(Decoder) }

func (d *Decoder) Decode(r io.Reader) (*File, error) {
	var raw map[string]json.RawMessage
	if err := json.NewDecoder(r).Decode(&raw); err != nil {
		return nil, fmt.Errorf("%w: %w", ErrMalformedJSON, err)
	}

	getString := func(key string) (string, error) {
		v, ok := raw[key]
		if !ok {
			return "", nil
		}
		var s string
		if err := json.Unmarshal(v, &s); err != nil {
			return "", fmt.Errorf(
				"%w: unmarshaling string for key %q: %w", ErrMalformedJSON, key, err,
			)
		}
		return s, nil
	}

	file := &File{
		CustomAttributes: map[string]any{},
		Messages:         map[string]Message{},
	}

	// Parse @@locale (required).
	localeStr, err := getString("@@locale")
	if err != nil {
		return nil, err
	}
	if localeStr == "" {
		return nil, ErrMissingRequiredLocale
	}

	locale, err := language.Parse(localeStr)
	if err != nil {
		return nil, fmt.Errorf("%w @@locale value %q: %w", ErrInvalid, localeStr, err)
	}
	file.Locale = locale

	// Optional fields.
	if file.Context, err = getString("@@context"); err != nil {
		return nil, err
	}
	if file.Author, err = getString("@@author"); err != nil {
		return nil, err
	}
	if file.Comment, err = getString("@@comment"); err != nil {
		return nil, err
	}

	// Parse @@last_modified.
	if rawLastMod, ok := raw["@@last_modified"]; ok {
		var s string
		if err := json.Unmarshal(rawLastMod, &s); err != nil {
			return nil, fmt.Errorf("%w: @@last_modified: %w", ErrMalformedJSON, err)
		}
		t, err := time.Parse(time.RFC3339, s)
		if err != nil {
			return nil, fmt.Errorf("%w @@last_modified format: %w", ErrInvalid, err)
		}
		file.LastModified = t
	}

	// Parse @@x-* attributes.
	for k, v := range raw {
		if strings.HasPrefix(k, "@@x-") {
			var anyVal any
			if err := json.Unmarshal(v, &anyVal); err != nil {
				return nil, fmt.Errorf(
					"%w: custom attribute %q: %w", ErrMalformedJSON, k, err,
				)
			}
			file.CustomAttributes[k] = anyVal
		}
	}

	// Parse messages.
	for k, v := range raw {
		if strings.HasPrefix(k, "@@") || strings.HasPrefix(k, "@") {
			continue // Skip metadata keys.
		}

		var msgText string
		if err := json.Unmarshal(v, &msgText); err != nil {
			return nil, fmt.Errorf(
				"%w: message text for key %q: %w", ErrMalformedJSON, k, err,
			)
		}

		if strings.TrimSpace(msgText) == "" {
			return nil, fmt.Errorf("for key %q: %w", k, ErrEmptyICUMsg)
		}

		msg := Message{
			ID:         k,
			ICUMessage: msgText,
		}

		d.buffer = d.buffer[:0] // Reuse buffer.
		d.buffer, err = d.tokenizer.Tokenize(locale, d.buffer, msgText)
		if err != nil {
			return nil, fmt.Errorf("%w: at index %d: %w",
				ErrInvalidICUMessage, d.tokenizer.Pos(), err)
		}
		msg.ICUMessageTokens = make([]icumsg.Token, len(d.buffer))
		copy(msg.ICUMessageTokens, d.buffer)

		// Check for @key metadata.
		metaKey := "@" + k
		if metaRaw, ok := raw[metaKey]; ok {
			var meta struct {
				Description  string                 `json:"description"`
				Comment      string                 `json:"comment"`
				Type         string                 `json:"type"`
				Context      string                 `json:"context"`
				Placeholders map[string]Placeholder `json:"placeholders"`
			}
			if err := json.Unmarshal(metaRaw, &meta); err != nil {
				return nil, fmt.Errorf(
					"%w: metadata for key %q: %w", ErrMalformedJSON, k, err,
				)
			}

			if meta.Type != "" {
				if err := validateMessageType(meta.Type); err != nil {
					return nil, fmt.Errorf("%w message type: %w", ErrInvalid, err)
				}
			} else {
				meta.Type = string(MessageTypeText) // Use default.
			}

			msg.Description = meta.Description
			msg.Comment = meta.Comment
			msg.Type = MessageType(meta.Type)
			msg.Context = meta.Context
			msg.Placeholders = meta.Placeholders

			for k, p := range meta.Placeholders {
				if p.Type == "" {
					continue
				}
				if err := validatePlaceholderType(string(p.Type)); err != nil {
					return nil, fmt.Errorf(
						"%w (for key %q): %w", ErrInvalid, k, err)
				}
			}

			for _, tok := range msg.ICUMessageTokens {
				if tok.Type == icumsg.TokenTypeArgName {
					name := tok.String(msgText, msg.ICUMessageTokens)
					if _, ok := meta.Placeholders[name]; !ok {
						return nil, fmt.Errorf("%w: %q", ErrUndefinedPlaceholder, name)
					}
				}
			}

			var attribute map[string]json.RawMessage
			if err := json.Unmarshal(metaRaw, &attribute); err != nil {
				return nil, fmt.Errorf(
					"%w: metadata for key %q: %w", ErrMalformedJSON, k, err,
				)
			}

			msg.CustomAttributes = make(map[string]any)
			for k, raw := range attribute {
				if !strings.HasPrefix(k, "x-") {
					continue
				}
				var v any
				if err := json.Unmarshal(raw, &v); err != nil {
					return nil, fmt.Errorf(
						"%w: custom attribute for key %q: %w", ErrMalformedJSON, k, err,
					)
				}
				msg.CustomAttributes[k] = v
			}
		}

		file.Messages[k] = msg
	}

	return file, nil
}

func validateMessageType(s string) error {
	switch MessageType(s) {
	case MessageTypeText, MessageTypeImage, MessageTypeCSS:
		// valid
		return nil
	}
	return fmt.Errorf("unsupported message type: %q", s)
}

func validatePlaceholderType(s string) error {
	switch PlaceholderType(s) {
	case PlaceholderString,
		PlaceholderInt,
		PlaceholderDouble,
		PlaceholderNum,
		PlaceholderDateTime:
		return nil
	}
	return fmt.Errorf("unsupported placeholder type: %q", s)
}

func Encode(w io.Writer, file *File, indent string) error {
	type encodedPlaceholder struct {
		Type               string         `json:"type,omitempty"`
		Description        string         `json:"description,omitempty"`
		Example            string         `json:"example,omitempty"`
		Format             string         `json:"format,omitempty"`
		IsCustomDateFormat bool           `json:"isCustomDateFormat,omitempty"`
		OptionalParameters map[string]any `json:"optionalParameters,omitempty"`
	}

	type kv struct {
		Key   string
		Value any
	}

	var entries []kv

	// Global metadata.
	entries = append(entries, kv{"@@locale", file.Locale.String()})
	if !file.LastModified.IsZero() {
		entries = append(entries, kv{"@@last_modified", file.LastModified.Format(time.RFC3339)})
	}
	if file.Context != "" {
		entries = append(entries, kv{"@@context", file.Context})
	}
	if file.Author != "" {
		entries = append(entries, kv{"@@author", file.Author})
	}
	if file.Comment != "" {
		entries = append(entries, kv{"@@comment", file.Comment})
	}

	for _, k := range slices.Sorted(maps.Keys(file.CustomAttributes)) {
		if strings.HasPrefix(k, "@@x-") {
			entries = append(entries, kv{k, file.CustomAttributes[k]})
		}
	}

	// Messages.
	for _, id := range slices.Sorted(maps.Keys(file.Messages)) {
		msg := file.Messages[id]
		entries = append(entries, kv{id, msg.ICUMessage})

		hasMeta := msg.Description != "" || msg.Comment != "" || msg.Type != "" ||
			msg.Context != "" || len(msg.Placeholders) > 0 || len(msg.CustomAttributes) > 0

		if !hasMeta {
			continue
		}

		// Build metadata in strict order.
		metaMap := make(map[string]any)

		if msg.Type != "" {
			metaMap["type"] = string(msg.Type)
		}
		if msg.Description != "" {
			metaMap["description"] = msg.Description
		}
		if msg.Comment != "" {
			metaMap["comment"] = msg.Comment
		}
		if msg.Context != "" {
			metaMap["context"] = msg.Context
		}
		if len(msg.Placeholders) > 0 {
			ph := make(map[string]encodedPlaceholder)
			for _, k := range slices.Sorted(maps.Keys(msg.Placeholders)) {
				v := msg.Placeholders[k]
				ph[k] = encodedPlaceholder{
					Type:               string(v.Type),
					Description:        v.Description,
					Example:            v.Example,
					Format:             v.Format,
					IsCustomDateFormat: v.IsCustomDateFormat,
					OptionalParameters: v.OptionalParameters,
				}
			}
			metaMap["placeholders"] = ph
		}

		for _, k := range slices.Sorted(maps.Keys(msg.CustomAttributes)) {
			if strings.HasPrefix(k, "x-") {
				metaMap[k] = msg.CustomAttributes[k]
			}
		}

		entries = append(entries, kv{"@" + id, metaMap})
	}

	// Output final JSON.
	_, _ = w.Write([]byte("{\n"))
	valBuf := &bytes.Buffer{}
	partColon := []byte(": ")
	partLineBreak := []byte("\n")
	partCommaLineBreak := []byte(",\n")
	enc := json.NewEncoder(valBuf)
	for i, entry := range entries {
		valBuf.Reset()
		keyJSON, err := json.Marshal(entry.Key)
		if err != nil {
			return err
		}

		enc.SetIndent(indent, indent)
		if err := enc.Encode(entry.Value); err != nil {
			return err
		}

		lines := strings.Split(strings.TrimRight(valBuf.String(), "\n"), "\n")

		_, _ = w.Write([]byte(indent))
		_, _ = w.Write([]byte(keyJSON))
		_, _ = w.Write(partColon)
		_, _ = w.Write([]byte(lines[0]))
		for _, line := range lines[1:] {
			_, _ = fmt.Fprintf(w, "\n%s", line)
		}

		if i < len(entries)-1 {
			_, _ = w.Write(partCommaLineBreak)
		} else {
			_, _ = w.Write(partLineBreak)
		}
	}
	_, err := w.Write([]byte("}\n"))
	return err
}
