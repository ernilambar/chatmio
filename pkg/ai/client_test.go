package ai

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestStreamChatParsesReasoningAndUsage(t *testing.T) {
	var request map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Errorf("decode request: %v", err)
		}
		w.Header().Set("Content-Type", "text/event-stream")
		for _, chunk := range []string{
			`{"choices":[{"delta":{"reasoning_content":"think "}}]}`,
			`{"choices":[{"delta":{"reasoning":"more"}}]}`,
			`{"choices":[{"delta":{"content":"Hi"}}]}`,
			`{"choices":[],"usage":{"prompt_tokens":12,"completion_tokens":34,"completion_tokens_details":{"reasoning_tokens":7}}}`,
		} {
			fmt.Fprintf(w, "data: %s\n\n", chunk)
		}
		fmt.Fprint(w, "data: [DONE]\n\n")
	}))
	defer server.Close()

	var content, reasoning string
	usage, err := New(server.URL, "", nil).StreamChat(context.Background(), ChatOptions{
		Model:           "m",
		ReasoningEffort: "high",
	}, func(delta Delta) {
		content += delta.Content
		reasoning += delta.Reasoning
	})
	if err != nil {
		t.Fatalf("StreamChat: %v", err)
	}
	if content != "Hi" || reasoning != "think more" {
		t.Fatalf("content = %q, reasoning = %q", content, reasoning)
	}
	if usage != (Usage{PromptTokens: 12, CompletionTokens: 34, ReasoningTokens: 7}) {
		t.Fatalf("usage = %+v", usage)
	}
	if request["reasoning_effort"] != "high" {
		t.Fatalf("reasoning_effort = %v, want high", request["reasoning_effort"])
	}
	if _, ok := request["temperature"]; ok {
		t.Fatalf("temperature sent although unset")
	}
	if _, ok := request["max_tokens"]; ok {
		t.Fatalf("max_tokens sent although unset")
	}
	if opts, _ := request["stream_options"].(map[string]any); opts["include_usage"] != true {
		t.Fatalf("stream_options = %v, want include_usage", request["stream_options"])
	}
}
