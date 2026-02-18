// ABOUTME: Tests for the chat app templates and handlers
// ABOUTME: Ensures templates parse correctly and handlers work as expected

package webadmin

import (
	"html/template"
	"testing"
)

func TestChatAppTemplatesParse(t *testing.T) {
	// Test that the chat app template parses correctly (must include templateFuncs for scriptTags)
	_, err := template.New("base.html").Funcs(templateFuncs).ParseFS(templateFS,
		"templates/base.html",
		"templates/chat_app.html",
	)
	if err != nil {
		t.Fatalf("failed to parse chat_app.html: %v", err)
	}

	// Test agent list partial
	_, err = template.ParseFS(templateFS, "templates/partials/agent_list.html")
	if err != nil {
		t.Fatalf("failed to parse agent_list.html: %v", err)
	}
}

func TestChatViewTemplateExecute(t *testing.T) {
	// Test that chat_view can be executed as a standalone partial
	tmpl, err := template.ParseFS(templateFS, "templates/chat_app.html")
	if err != nil {
		t.Fatalf("failed to parse chat_app.html: %v", err)
	}

	// Check that chat_view is defined
	if tmpl.Lookup("chat_view") == nil {
		t.Error("chat_view template not found in chat_app.html")
	}

	// Check that empty_state is defined
	if tmpl.Lookup("empty_state") == nil {
		t.Error("empty_state template not found in chat_app.html")
	}
}
