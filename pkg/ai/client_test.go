package ai

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
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

func TestStreamChatFallsBackToJSONResponse(t *testing.T) {
	var request map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Errorf("decode request: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{
			"choices": [{
				"message": {
					"content": "Fallback reply",
					"reasoning_content": "checked",
					"tool_calls": [{
						"id": "call_1",
						"type": "function",
						"function": {"name": "web_search", "arguments": "{\"query\":\"go\"}"}
					}]
				}
			}],
			"usage": {"prompt_tokens": 5, "completion_tokens": 9, "completion_tokens_details": {"reasoning_tokens": 2}}
		}`)
	}))
	defer server.Close()

	var content, reasoning string
	reply, err := New(server.URL, "", nil).StreamChat(context.Background(), ChatOptions{Model: "m"}, func(delta Delta) {
		content += delta.Content
		reasoning += delta.Reasoning
	})
	if err != nil {
		t.Fatalf("StreamChat: %v", err)
	}
	if content != "Fallback reply" || reasoning != "checked" {
		t.Fatalf("content = %q, reasoning = %q", content, reasoning)
	}
	if reply.Usage != (Usage{PromptTokens: 5, CompletionTokens: 9, ReasoningTokens: 2}) {
		t.Fatalf("usage = %+v", reply.Usage)
	}
	if len(reply.ToolCalls) != 1 || reply.ToolCalls[0].ID != "call_1" || reply.ToolCalls[0].Function.Name != "web_search" {
		t.Fatalf("tool calls = %+v", reply.ToolCalls)
	}
	if request["stream"] != true {
		t.Fatalf("stream = %v, want true", request["stream"])
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

func TestAPIErrorPrefersProviderMessage(t *testing.T) {
	body := `{"error":{"message":"Missing bearer authentication in header","type":"invalid_request_error"}}`
	resp := &http.Response{
		Status: "401 Unauthorized",
		Body:   io.NopCloser(strings.NewReader(body)),
	}
	if got, want := apiError(resp).Error(), "ai: 401 Unauthorized: Missing bearer authentication in header"; got != want {
		t.Fatalf("apiError = %q, want %q", got, want)
	}

	resp = &http.Response{Status: "502 Bad Gateway", Body: io.NopCloser(strings.NewReader("upstream down"))}
	if got, want := apiError(resp).Error(), "ai: 502 Bad Gateway: upstream down"; got != want {
		t.Fatalf("apiError = %q, want %q", got, want)
	}
}

func TestStreamChatStopsAfterIdleTimeout(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, "data: {\"choices\":[{\"delta\":{\"content\":\"Hi\"}}]}\n\n")
		w.(http.Flusher).Flush()
		// Never send another byte; the client must give up on its own.
		select {
		case <-r.Context().Done():
		case <-time.After(5 * time.Second):
		}
	}))
	defer server.Close()

	client := New(server.URL, "", nil)
	client.IdleTimeout = 100 * time.Millisecond

	var content strings.Builder
	_, err := client.StreamChat(context.Background(), ChatOptions{Model: "m"}, func(delta Delta) {
		content.WriteString(delta.Content)
	})
	if !errors.Is(err, ErrStreamStalled) {
		t.Fatalf("err = %v, want ErrStreamStalled", err)
	}
	if content.String() != "Hi" {
		t.Fatalf("content = %q, want Hi", content.String())
	}
	if !strings.Contains(err.Error(), "no data for") {
		t.Fatalf("err = %q, want idle duration", err)
	}
}

func TestStreamChatSendsCustomHeaders(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("x-opencode-session"); got != "session-1" {
			t.Errorf("x-opencode-session = %q, want session-1", got)
		}
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, "data: [DONE]\n\n")
	}))
	defer server.Close()

	headers := map[string]string{"x-opencode-session": "session-1"}
	if _, err := New(server.URL, "", headers).StreamChat(context.Background(), ChatOptions{Model: "m"}, func(Delta) {}); err != nil {
		t.Fatalf("StreamChat: %v", err)
	}
}

func sseServer(t *testing.T, body string) *httptest.Server {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, body)
	}))
	t.Cleanup(server.Close)
	return server
}

func TestStreamChatRejectsTruncatedStream(t *testing.T) {
	for name, body := range map[string]string{
		"partial": "data: {\"choices\":[{\"delta\":{\"content\":\"partial\"}}]}\n\n",
		"empty":   "",
		"no data": "<html>proxy error</html>\n",
	} {
		server := sseServer(t, body)
		_, err := New(server.URL, "", nil).StreamChat(context.Background(), ChatOptions{Model: "m"}, func(Delta) {})
		if !errors.Is(err, io.ErrUnexpectedEOF) {
			t.Errorf("%s: err = %v, want io.ErrUnexpectedEOF", name, err)
		}
	}
}

func TestStreamChatAcceptsFinishReasonWithoutDone(t *testing.T) {
	server := sseServer(t, "data: {\"choices\":[{\"delta\":{\"content\":\"ok\"},\"finish_reason\":\"stop\"}]}\n\n")
	var content strings.Builder
	_, err := New(server.URL, "", nil).StreamChat(context.Background(), ChatOptions{Model: "m"}, func(d Delta) {
		content.WriteString(d.Content)
	})
	if err != nil || content.String() != "ok" {
		t.Fatalf("content = %q, err = %v; want ok, nil", content.String(), err)
	}
}

func TestStreamChatRejectsOutOfRangeToolIndex(t *testing.T) {
	for _, index := range []int{-1, MaxToolCalls, 100000} {
		server := sseServer(t, fmt.Sprintf(
			"data: {\"choices\":[{\"delta\":{\"tool_calls\":[{\"index\":%d,\"id\":\"a\",\"function\":{\"name\":\"x\"}}]}}]}\n\ndata: [DONE]\n\n", index))
		reply, err := New(server.URL, "", nil).StreamChat(context.Background(), ChatOptions{Model: "m"}, func(Delta) {})
		if err == nil || len(reply.ToolCalls) != 0 {
			t.Errorf("index %d: calls = %d, err = %v; want an error and no calls", index, len(reply.ToolCalls), err)
		}
	}
}

func TestStreamChatRejectsCrossOriginRedirect(t *testing.T) {
	replayed := false
	other := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		replayed = true
	}))
	defer other.Close()
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, other.URL+r.URL.Path, http.StatusTemporaryRedirect)
	}))
	defer provider.Close()

	_, err := New(provider.URL, "key", nil).StreamChat(context.Background(), ChatOptions{Model: "m"}, func(Delta) {})
	if err == nil || !strings.Contains(err.Error(), "refusing redirect") {
		t.Fatalf("err = %v, want redirect refusal", err)
	}
	if replayed {
		t.Fatal("prompt body was replayed to another origin")
	}
}
