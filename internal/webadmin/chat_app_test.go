// ABOUTME: Tests for the chat app templates
// ABOUTME: Ensures the v2 Svelte island template parses correctly

package webadmin

import (
	"html/template"
	"testing"
)

func TestChatAppV2TemplateParse(t *testing.T) {
	_, err := template.New("base.html").Funcs(templateFuncs).ParseFS(templateFS,
		"templates/base.html",
		"templates/chat_app_v2.html",
	)
	if err != nil {
		t.Fatalf("failed to parse chat_app_v2.html: %v", err)
	}
}
