package bootstrap_test

import (
	"strings"
	"testing"
	"time"

	"github.com/pradita/ac-project/service/api/internal/bootstrap"
)

const fieldEmail = "email"

func TestInputValidate(t *testing.T) {
	t.Parallel()

	good := bootstrap.Input{
		Tenant:       "Pradita",
		Organization: "HQ",
		Email:        "admin@example.com",
		Name:         "Admin",
		At:           time.Time{},
	}

	cases := []struct {
		name   string
		mutate func(in *bootstrap.Input)
		want   string // a prefix of the error; empty: valid
	}{
		{"valid", func(*bootstrap.Input) {}, ""},
		{"blank tenant", func(in *bootstrap.Input) { in.Tenant = "  " }, "tenant"},
		{
			"long tenant",
			func(in *bootstrap.Input) { in.Tenant = strings.Repeat("あ", 121) },
			"tenant",
		},
		{
			"120 multibyte characters",
			func(in *bootstrap.Input) { in.Tenant = strings.Repeat("あ", 120) },
			"",
		},
		{"blank organization", func(in *bootstrap.Input) { in.Organization = "" }, "organization"},
		{
			"long organization",
			func(in *bootstrap.Input) { in.Organization = strings.Repeat("あ", 121) },
			"organization",
		},
		{
			"no at sign",
			func(in *bootstrap.Input) { in.Email = "admin.example.com" },
			fieldEmail,
		},
		{"no domain dot", func(in *bootstrap.Input) { in.Email = "admin@example" }, fieldEmail},
		{
			"space in address",
			func(in *bootstrap.Input) { in.Email = "a b@example.com" },
			fieldEmail,
		},
		{
			"254-character address",
			func(in *bootstrap.Input) { in.Email = strings.Repeat("a", 248) + "@e.com" },
			"",
		},
		{
			"long address",
			func(in *bootstrap.Input) { in.Email = strings.Repeat("a", 249) + "@e.com" },
			fieldEmail,
		},
		{"blank name", func(in *bootstrap.Input) { in.Name = "\t" }, "name"},
		{
			"long name",
			func(in *bootstrap.Input) { in.Name = strings.Repeat("あ", 121) },
			"name",
		},
		{
			"120-character organization and name",
			func(in *bootstrap.Input) {
				in.Organization, in.Name = strings.Repeat("あ", 120), strings.Repeat("あ", 120)
			},
			"",
		},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			in := good
			testCase.mutate(&in)

			err := in.Validate()

			switch {
			case testCase.want == "" && err != nil:
				t.Fatalf("Validate() = %v, want nil", err)
			case testCase.want != "" && (err == nil || !strings.HasPrefix(err.Error(), testCase.want)):
				t.Fatalf("Validate() = %v, want an error about %q", err, testCase.want)
			}
		})
	}
}

func TestInputValidateTrims(t *testing.T) {
	t.Parallel()

	input := bootstrap.Input{
		Tenant:       " T ",
		Organization: " O ",
		Email:        " a@b.co ",
		Name:         " N ",
		At:           time.Time{},
	}

	err := input.Validate()
	if err != nil {
		t.Fatal(err)
	}

	if input.Tenant != "T" || input.Organization != "O" || input.Email != "a@b.co" ||
		input.Name != "N" {
		t.Fatalf("not trimmed: %+v", input)
	}
}
