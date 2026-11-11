package acp

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"regexp"
	"slices"
	"unicode/utf8"
)

// Form elicitation (ACP v1, unstable): the agent asks the user to fill in a
// form, which is how Claude Code's AskUserQuestion, Codex's
// request_user_input and OpenCode's question tool reach a client.
// docs/plan/chat-questions-adapters.md describes what each adapter sends.

// Methods of form elicitation.
const (
	MethodElicitationCreate = "elicitation/create"
	MethodCancelRequest     = "$/cancel_request"
)

// ErrWithdrawn is the context.Cause of a request handler's context when the
// agent withdrew the request with $/cancel_request.
var ErrWithdrawn = errors.New("acp: the agent withdrew the request")

// CodeRequestCancelled answers a request the agent withdrew with
// $/cancel_request.
const CodeRequestCancelled = -32800

// Elicitation actions.
const (
	ElicitationAccept  = "accept"
	ElicitationDecline = "decline"
	ElicitationCancel  = "cancel"
)

// ElicitationCapabilities says which elicitation modes the client shows. A
// present Form means form mode.
type ElicitationCapabilities struct {
	Form *struct{} `json:"form,omitempty"`
}

// CancelRequestNotification is the params of $/cancel_request.
type CancelRequestNotification struct {
	RequestID json.RawMessage `json:"requestId"`
}

// CreateElicitationRequest is the params of elicitation/create. Only
// session-scoped form mode is shown; RequestID scopes a request outside a
// session, such as a login.
type CreateElicitationRequest struct {
	SessionID       string             `json:"sessionId,omitempty"`
	ToolCallID      string             `json:"toolCallId,omitempty"`
	RequestID       json.RawMessage    `json:"requestId,omitempty"`
	Mode            string             `json:"mode"`
	Message         string             `json:"message"`
	RequestedSchema *ElicitationSchema `json:"requestedSchema,omitempty"`
	Meta            json.RawMessage    `json:"_meta,omitempty"`
}

// CreateElicitationResponse answers elicitation/create.
type CreateElicitationResponse struct {
	Action  string         `json:"action"`
	Content map[string]any `json:"content,omitempty"`
}

// ElicitationCancelled is the answer to a form the user never answered:
// the turn was cancelled, the chat closed or the agent withdrew it.
func ElicitationCancelled() CreateElicitationResponse {
	return CreateElicitationResponse{Action: ElicitationCancel}
}

// ElicitationSchema is a form: an object of flat properties.
type ElicitationSchema struct {
	Type        string            `json:"type,omitempty"`
	Title       string            `json:"title,omitempty"`
	Description string            `json:"description,omitempty"`
	Properties  ElicitationFields `json:"properties"`
	Required    []string          `json:"required,omitempty"`
	Meta        json.RawMessage   `json:"_meta,omitempty"`
}

// ElicitationFields are a form's properties in the order the agent sent
// them, which is the order the user reads the questions in.
type ElicitationFields []ElicitationField

// ElicitationField is one named property.
type ElicitationField struct {
	Key      string              `json:"key"`
	Property ElicitationProperty `json:"property"`
}

// ElicitationProperty is one field of a form. Unknown names a key this
// client does not know, which makes the field unsupported.
type ElicitationProperty struct {
	Type        string            `json:"type"`
	Title       string            `json:"title,omitempty"`
	Description string            `json:"description,omitempty"`
	Default     json.RawMessage   `json:"default,omitempty"`
	Enum        []string          `json:"enum,omitempty"`
	OneOf       []EnumOption      `json:"oneOf,omitempty"`
	MinLength   *int              `json:"minLength,omitempty"`
	MaxLength   *int              `json:"maxLength,omitempty"`
	Pattern     string            `json:"pattern,omitempty"`
	Format      string            `json:"format,omitempty"`
	Minimum     *float64          `json:"minimum,omitempty"`
	Maximum     *float64          `json:"maximum,omitempty"`
	Items       *ElicitationItems `json:"items,omitempty"`
	MinItems    *int              `json:"minItems,omitempty"`
	MaxItems    *int              `json:"maxItems,omitempty"`
	Meta        json.RawMessage   `json:"_meta,omitempty"`
	Unknown     []string          `json:"-"`
}

// ElicitationItems are a multi-select's options.
type ElicitationItems struct {
	Type  string          `json:"type,omitempty"`
	Enum  []string        `json:"enum,omitempty"`
	AnyOf []EnumOption    `json:"anyOf,omitempty"`
	Meta  json.RawMessage `json:"_meta,omitempty"`
}

// EnumOption is a titled option.
type EnumOption struct {
	Const       string          `json:"const"`
	Title       string          `json:"title"`
	Description string          `json:"description,omitempty"`
	Meta        json.RawMessage `json:"_meta,omitempty"`
}

var propertyKeys = map[string]bool{
	"type": true, "title": true, "description": true, "default": true, "enum": true, "oneOf": true,
	"minLength": true, "maxLength": true, "pattern": true, "format": true, "minimum": true, "maximum": true,
	"items": true, "minItems": true, "maxItems": true, "_meta": true,
}

// UnmarshalJSON keeps the properties in the order they were sent.
func (f *ElicitationFields) UnmarshalJSON(b []byte) error {
	dec := json.NewDecoder(bytes.NewReader(b))
	tok, err := dec.Token()
	if err != nil {
		return err
	}
	if tok == nil {
		*f = nil
		return nil
	}
	if d, ok := tok.(json.Delim); !ok || d != '{' {
		return errors.New("acp: form properties are not an object")
	}
	var out ElicitationFields
	for dec.More() {
		tok, err := dec.Token()
		if err != nil {
			return err
		}
		key, _ := tok.(string)
		var raw json.RawMessage
		if err := dec.Decode(&raw); err != nil {
			return err
		}
		var p ElicitationProperty
		if err := json.Unmarshal(raw, &p); err != nil {
			return fmt.Errorf("acp: form property %q: %w", key, err)
		}
		var keys map[string]json.RawMessage
		if err := json.Unmarshal(raw, &keys); err != nil {
			return fmt.Errorf("acp: form property %q: %w", key, err)
		}
		for k := range keys {
			if !propertyKeys[k] {
				p.Unknown = append(p.Unknown, k)
			}
		}
		slices.Sort(p.Unknown)
		out = append(out, ElicitationField{Key: key, Property: p})
	}
	*f = out
	return nil
}

// MarshalJSON writes the properties as an object, in order.
func (f ElicitationFields) MarshalJSON() ([]byte, error) {
	var b bytes.Buffer
	b.WriteByte('{')
	for i, field := range f {
		if i > 0 {
			b.WriteByte(',')
		}
		k, err := json.Marshal(field.Key)
		if err != nil {
			return nil, err
		}
		v, err := json.Marshal(field.Property)
		if err != nil {
			return nil, err
		}
		b.Write(k)
		b.WriteByte(':')
		b.Write(v)
	}
	b.WriteByte('}')
	return b.Bytes(), nil
}

// Field returns the property named key.
func (s *ElicitationSchema) Field(key string) (ElicitationProperty, bool) {
	for _, f := range s.Properties {
		if f.Key == key {
			return f.Property, true
		}
	}
	return ElicitationProperty{}, false
}

// Supported reports why a form cannot be shown in full, or nil when it can.
func (s *ElicitationSchema) Supported() error {
	if s == nil {
		return errors.New("the form has no schema")
	}
	if s.Type != "" && s.Type != "object" {
		return fmt.Errorf("the form is a %q, not an object", s.Type)
	}
	seen := map[string]bool{}
	for _, f := range s.Properties {
		if f.Key == "" {
			return errors.New("a form field has no name")
		}
		if seen[f.Key] {
			return fmt.Errorf("the form field %q appears twice", f.Key)
		}
		seen[f.Key] = true
		if err := f.Property.supported(); err != nil {
			return fmt.Errorf("the form field %q: %w", f.Key, err)
		}
	}
	for _, k := range s.Required {
		if !seen[k] {
			return fmt.Errorf("the form requires %q, which it does not have", k)
		}
	}
	return nil
}

func (p ElicitationProperty) supported() error {
	if len(p.Unknown) > 0 {
		return fmt.Errorf("unknown keys %q", p.Unknown)
	}
	switch p.Type {
	case "string":
		if p.Format != "" {
			return fmt.Errorf("the format %q is not supported", p.Format)
		}
		if p.Pattern != "" {
			if _, err := regexp.Compile(p.Pattern); err != nil {
				return fmt.Errorf("the pattern %q: %w", p.Pattern, err)
			}
		}
		if len(p.OneOf) > 0 || len(p.Enum) > 0 {
			return checkOptions(p.options())
		}
		return nil
	case "array":
		if p.Items == nil || (len(p.Items.AnyOf) == 0 && len(p.Items.Enum) == 0) {
			return errors.New("a list without options is not supported")
		}
		if p.Items.Type != "" && p.Items.Type != "string" {
			return fmt.Errorf("a list of %q is not supported", p.Items.Type)
		}
		return checkOptions(p.options())
	case "boolean", "number", "integer":
		return nil
	}
	return fmt.Errorf("the type %q is not supported", p.Type)
}

func checkOptions(opts []EnumOption) error {
	seen := map[string]bool{}
	for _, o := range opts {
		if seen[o.Const] {
			return fmt.Errorf("the option %q appears twice", o.Const)
		}
		seen[o.Const] = true
	}
	return nil
}

// options are a choice's or a multi-select's options, titled.
func (p ElicitationProperty) options() []EnumOption {
	var opts []EnumOption
	var values []string
	switch {
	case p.Type == "string" && len(p.OneOf) > 0:
		opts = p.OneOf
	case p.Type == "string":
		values = p.Enum
	case p.Items != nil && len(p.Items.AnyOf) > 0:
		opts = p.Items.AnyOf
	case p.Items != nil:
		values = p.Items.Enum
	}
	for _, v := range values {
		opts = append(opts, EnumOption{Const: v, Title: v})
	}
	return opts
}

func (p ElicitationProperty) hasOption(v string) bool {
	return slices.ContainsFunc(p.options(), func(o EnumOption) bool { return o.Const == v })
}

// Validate checks an accepted form's content against the schema and returns
// it with each value in its JSON type. A key the form does not have, a
// missing required field or a value out of bounds is an error.
func (s *ElicitationSchema) Validate(content map[string]json.RawMessage) (map[string]any, error) {
	if err := s.Supported(); err != nil {
		return nil, err
	}
	out := make(map[string]any, len(content))
	for key, raw := range content {
		p, ok := s.Field(key)
		if !ok {
			return nil, fmt.Errorf("the form has no field %q", key)
		}
		// No value, or empty text, is no answer.
		if len(raw) == 0 || string(raw) == "null" || string(raw) == `""` {
			continue
		}
		v, err := p.value(raw)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", fieldName(key, p), err)
		}
		out[key] = v
	}
	for _, key := range s.Required {
		if _, ok := out[key]; !ok {
			p, _ := s.Field(key)
			return nil, fmt.Errorf("%s needs an answer", fieldName(key, p))
		}
	}
	return out, nil
}

// ValidateValues is Validate for content already decoded from JSON, or
// built in Go.
func (s *ElicitationSchema) ValidateValues(content map[string]any) (map[string]any, error) {
	raw := make(map[string]json.RawMessage, len(content))
	for k, v := range content {
		b, err := json.Marshal(v)
		if err != nil {
			return nil, fmt.Errorf("%q: %w", k, err)
		}
		raw[k] = b
	}
	return s.Validate(raw)
}

func fieldName(key string, p ElicitationProperty) string {
	if p.Title != "" {
		return fmt.Sprintf("%q", p.Title)
	}
	return fmt.Sprintf("%q", key)
}

func (p ElicitationProperty) value(raw json.RawMessage) (any, error) {
	switch p.Type {
	case "string":
		var v string
		if err := json.Unmarshal(raw, &v); err != nil {
			return nil, errors.New("the answer is not text")
		}
		if len(p.OneOf) > 0 || len(p.Enum) > 0 {
			if !p.hasOption(v) {
				return nil, fmt.Errorf("%q is not one of the options", v)
			}
			return v, nil
		}
		n := utf8.RuneCountInString(v)
		if p.MinLength != nil && n < *p.MinLength {
			return nil, fmt.Errorf("the answer needs at least %d characters", *p.MinLength)
		}
		if p.MaxLength != nil && n > *p.MaxLength {
			return nil, fmt.Errorf("the answer may have at most %d characters", *p.MaxLength)
		}
		if p.Pattern != "" {
			if re, err := regexp.Compile(p.Pattern); err != nil || !re.MatchString(v) {
				return nil, errors.New("the answer does not have the expected form")
			}
		}
		return v, nil
	case "array":
		var v []string
		if err := json.Unmarshal(raw, &v); err != nil {
			return nil, errors.New("the answer is not a list of options")
		}
		seen := map[string]bool{}
		for _, item := range v {
			if !p.hasOption(item) {
				return nil, fmt.Errorf("%q is not one of the options", item)
			}
			if seen[item] {
				return nil, fmt.Errorf("%q is chosen twice", item)
			}
			seen[item] = true
		}
		if p.MinItems != nil && len(v) < *p.MinItems {
			return nil, fmt.Errorf("choose at least %d", *p.MinItems)
		}
		if p.MaxItems != nil && len(v) > *p.MaxItems {
			return nil, fmt.Errorf("choose at most %d", *p.MaxItems)
		}
		if v == nil {
			v = []string{}
		}
		return v, nil
	case "boolean":
		var v bool
		if err := json.Unmarshal(raw, &v); err != nil {
			return nil, errors.New("the answer is not yes or no")
		}
		return v, nil
	case "number", "integer":
		var v float64
		if err := json.Unmarshal(raw, &v); err != nil || math.IsInf(v, 0) || math.IsNaN(v) {
			return nil, errors.New("the answer is not a number")
		}
		if p.Type == "integer" && v != math.Trunc(v) {
			return nil, errors.New("the answer is not a whole number")
		}
		if p.Minimum != nil && v < *p.Minimum {
			return nil, fmt.Errorf("the answer is below %v", *p.Minimum)
		}
		if p.Maximum != nil && v > *p.Maximum {
			return nil, fmt.Errorf("the answer is above %v", *p.Maximum)
		}
		if p.Type == "integer" {
			return int64(v), nil
		}
		return v, nil
	}
	return nil, fmt.Errorf("the type %q is not supported", p.Type)
}
