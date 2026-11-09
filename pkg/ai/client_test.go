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
	reply, err := New(server.URL, "", nil).StreamChat(context.Background(), ChatOptions{
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
	if reply.Usage != (Usage{PromptTokens: 12, CompletionTokens: 34, ReasoningTokens: 7}) {
		t.Fatalf("usage = %+v", reply.Usage)
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

func TestStreamChatCollectsToolCalls(t *testing.T) {
	var request map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Errorf("decode request: %v", err)
		}
		w.Header().Set("Content-Type", "text/event-stream")
		for _, chunk := range []string{
			`{"choices":[{"delta":{"tool_calls":[{"index":0,"id":"a","type":"function","function":{"name":"web_search","arguments":""}}]}}]}`,
			`{"choices":[{"delta":{"tool_calls":[{"index":0,"function":{"arguments":"{\"query\":"}}]}}]}`,
			`{"choices":[{"delta":{"tool_calls":[{"index":1,"id":"b","function":{"name":"web_fetch","arguments":"{\"url\":\"https://x\"}"}}]}}]}`,
			`{"choices":[{"delta":{"tool_calls":[{"index":0,"function":{"arguments":"\"go\"}"}}]}}]}`,
		} {
			fmt.Fprintf(w, "data: %s\n\n", chunk)
		}
		fmt.Fprint(w, "data: [DONE]\n\n")
	}))
	defer server.Close()

	reply, err := New(server.URL, "", nil).StreamChat(context.Background(), ChatOptions{
		Model: "m",
		Tools: []Tool{{Type: "function", Function: ToolFunction{Name: "web_search", Parameters: map[string]any{}}}},
	}, func(Delta) {})
	if err != nil {
		t.Fatalf("StreamChat: %v", err)
	}
	want := []ToolCall{
		{ID: "a", Type: "function", Function: ToolCallFunction{Name: "web_search", Arguments: `{"query":"go"}`}},
		{ID: "b", Type: "function", Function: ToolCallFunction{Name: "web_fetch", Arguments: `{"url":"https://x"}`}},
	}
	if fmt.Sprint(reply.ToolCalls) != fmt.Sprint(want) {
		t.Fatalf("tool calls = %+v, want %+v", reply.ToolCalls, want)
	}
	if tools, _ := request["tools"].([]any); len(tools) != 1 {
		t.Fatalf("tools = %v, want one tool", request["tools"])
	}
}
