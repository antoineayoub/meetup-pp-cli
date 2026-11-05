package cli

import (
	"strings"
	"testing"
)

func TestNormalizeNotionDescriptionBuildsMeetupParagraphsAndLists(t *testing.T) {
	input := "First paragraph\nSecond paragraph\n### What you'll learn\n- First takeaway\n- Second takeaway\n<empty-block/>"
	got, err := normalizeNotionDescription(input)
	if err != nil {
		t.Fatal(err)
	}
	want := "First paragraph\n\nSecond paragraph\n\n**What you'll learn**\n\n• First takeaway\n• Second takeaway"
	if got != want {
		t.Fatalf("normalized description = %q, want %q", got, want)
	}
}

func TestNormalizeNotionDescriptionConvertsLiteralNewlines(t *testing.T) {
	got, err := normalizeNotionDescription(`First\n\nSecond`)
	if err != nil {
		t.Fatal(err)
	}
	if got != "First\n\nSecond" {
		t.Fatalf("normalized description = %q", got)
	}
}

func TestNormalizeNotionDescriptionRejectsCodeFences(t *testing.T) {
	_, err := normalizeNotionDescription("```markdown\nText\n```")
	if err == nil || !strings.Contains(err.Error(), "code fences") {
		t.Fatalf("expected code fence error, got %v", err)
	}
}

func TestHybridMeetupTargetsAndFormats(t *testing.T) {
	groups, err := notionTargetGroups("Hybrid", "auto")
	if err != nil {
		t.Fatal(err)
	}
	if got, want := strings.Join(groups, ","), "le-wagon-montreal,le-wagon-toronto"; got != want {
		t.Fatalf("groups = %q, want %q", got, want)
	}

	for group, want := range map[string]string{
		"le-wagon-montreal": "onsite",
		"le-wagon-toronto":  "online",
	} {
		got, err := meetupFormatForGroup("Hybrid", group)
		if err != nil || got != want {
			t.Fatalf("Hybrid format for %s = %q, %v; want %q", group, got, err, want)
		}
	}
}
