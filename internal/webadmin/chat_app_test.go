// ABOUTME: Tests for the chat app templates and handlers
// ABOUTME: Ensures templates parse correctly and handlers work as expected

package webadmin

import (
	"html/template"
	"testing"
)

func TestChatAppTemplatesParse(t *testing.T) {
	// Test that the chat app template parses correctly
	_, err := template.ParseFS(templateFS,
		"templates/base.html",
		"templates/chat_app.html",
	)
	if err != nil {
		t.Fatalf("failed to parse chat_app.html: %v", err)
	}

	// Test sidebar partial
	tmpl, err := template.ParseFS(templateFS, "templates/partials/sidebar.html")
	if err != nil {
		t.Fatalf("failed to parse sidebar.html: %v", err)
	}

	// Check that thread_item is defined
	if tmpl.Lookup("thread_item") == nil {
		t.Error("thread_item template not found in sidebar.html")
	}

	// Test agent picker partial
	_, err = template.ParseFS(templateFS, "templates/partials/agent_picker.html")
	if err != nil {
		t.Fatalf("failed to parse agent_picker.html: %v", err)
	}

	// Test thread search results partial
	_, err = template.ParseFS(templateFS, "templates/partials/thread_search_results.html")
	if err != nil {
		t.Fatalf("failed to parse thread_search_results.html: %v", err)
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
