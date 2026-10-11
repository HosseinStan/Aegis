package domain

import (
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestChangeAuthorizationInputValidation(t *testing.T) {
	now := time.Date(2026, 10, 12, 8, 0, 0, 0, time.UTC)
	valid := func() ChangeAuthorizationInput {
		return ChangeAuthorizationInput{Reference: " chg-2026-00124 ", ConfigType: "running", AllowedCategories: []string{" Switching ", "interface"}, WindowStart: now, WindowEnd: now.Add(2 * time.Hour)}
	}
	got, err := NormalizeChangeAuthorizationInput(valid(), now)
	require.NoError(t, err)
	require.Equal(t, "CHG-2026-00124", got.Reference)
	require.Equal(t, []string{"interface", "switching"}, got.AllowedCategories)
	for _, test := range []struct {
		name string
		edit func(*ChangeAuthorizationInput)
	}{
		{"empty reference", func(i *ChangeAuthorizationInput) { i.Reference = "" }},
		{"short reference", func(i *ChangeAuthorizationInput) { i.Reference = "AB" }},
		{"long reference", func(i *ChangeAuthorizationInput) { i.Reference = strings.Repeat("A", 65) }},
		{"spaces", func(i *ChangeAuthorizationInput) { i.Reference = "CHG 123" }},
		{"newline", func(i *ChangeAuthorizationInput) { i.Reference = "CHG\n123" }},
		{"trailing newline", func(i *ChangeAuthorizationInput) { i.Reference = "CHG-123\n" }},
		{"non ASCII", func(i *ChangeAuthorizationInput) { i.Reference = "CHG-é" }},
		{"Unicode ASCII lookalike", func(i *ChangeAuthorizationInput) { i.Reference = "CHG-ı" }},
		{"SQL injection", func(i *ChangeAuthorizationInput) { i.Reference = "CHG';DROP TABLE users;--" }},
		{"config type", func(i *ChangeAuthorizationInput) { i.ConfigType = "candidate" }},
		{"empty categories", func(i *ChangeAuthorizationInput) { i.AllowedCategories = nil }},
		{"duplicates", func(i *ChangeAuthorizationInput) { i.AllowedCategories = []string{"ROUTING", " routing "} }},
		{"unknown category", func(i *ChangeAuthorizationInput) { i.AllowedCategories = []string{"wildcard"} }},
		{"unclassified", func(i *ChangeAuthorizationInput) { i.AllowedCategories = []string{"unclassified"} }},
		{"wildcard", func(i *ChangeAuthorizationInput) { i.AllowedCategories = []string{"*"} }},
		{"reversed window", func(i *ChangeAuthorizationInput) { i.WindowStart = i.WindowEnd.Add(time.Second) }},
		{"equal window", func(i *ChangeAuthorizationInput) { i.WindowEnd = i.WindowStart }},
		{"expired", func(i *ChangeAuthorizationInput) { i.WindowStart = now.Add(-time.Hour); i.WindowEnd = now }},
		{"over 24 hours", func(i *ChangeAuthorizationInput) { i.WindowEnd = i.WindowStart.Add(24*time.Hour + time.Nanosecond) }},
		{"missing timestamp", func(i *ChangeAuthorizationInput) { i.WindowStart = time.Time{} }},
	} {
		t.Run(test.name, func(t *testing.T) {
			in := valid()
			test.edit(&in)
			_, err := NormalizeChangeAuthorizationInput(in, now)
			require.ErrorIs(t, err, ErrValidation)
		})
	}
	in := valid()
	in.AllowedCategories = []string{"other"}
	in.WindowEnd = in.WindowStart.Add(24 * time.Hour)
	_, err = NormalizeChangeAuthorizationInput(in, now)
	require.NoError(t, err)
	offset := time.FixedZone("explicit-offset", 3*60*60+30*60)
	in.WindowStart = in.WindowStart.In(offset)
	in.WindowEnd = in.WindowEnd.In(offset)
	got, err = NormalizeChangeAuthorizationInput(in, now)
	require.NoError(t, err)
	require.Equal(t, time.UTC, got.WindowStart.Location())
	require.True(t, got.WindowStart.Equal(now))
}
