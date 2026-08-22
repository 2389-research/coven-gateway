// ABOUTME: Tests for the chat app templates
// ABOUTME: Ensures the Svelte island template parses correctly

package webadmin

import (
	"html/template"
	"testing"
)

func TestChatAppTemplateParse(t *testing.T) {
	_, err := template.New("base.html").Funcs(templateFuncs).ParseFS(templateFS,
		"templates/base.html",
		"templates/chat_app.html",
	)
	if err != nil {
		t.Fatalf("failed to parse chat_app.html: %v", err)
	}
}
