package issue

import (
	"strings"
	"testing"
)

func TestReference(t *testing.T) {
	tests := []struct {
		name string
		text string
		want string
	}{
		{"a bare key", "TICKET-2568", "TICKET-2568"},
		{"a key with whitespace around it", "  TICKET-2568\n", "TICKET-2568"},
		{"a browse URL", "https://example.atlassian.net/browse/TICKET-2568", "TICKET-2568"},
		{"a browse URL with a query", "https://x.atlassian.net/browse/AB-1?filter=2", "AB-1"},
		{"a project with digits in it", "H2O-9", "H2O-9"},

		// A loose match would resolve its own output and never settle.
		{"a branch name holding a key", "fix/TICKET-2568-billing-dashboard", ""},
		{"a branch name", "feat/kafka-retries", ""},
		{"a lowercase key, as typed", "ticket-2568", "ticket-2568"},
		{"a lowercase browse URL", "https://x.atlassian.net/browse/ab-1", "ab-1"},
		{"a URL that is not a browse page", "https://github.com/o/r/pull/12", ""},
		{"nothing at all", "", ""},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, ok := Reference(test.text)

			if ok != (test.want != "") {
				t.Fatalf("Reference(%q) matched = %v, want %v", test.text, ok, test.want != "")
			}
			if got != test.want {
				t.Fatalf("Reference(%q) = %q, want %q", test.text, got, test.want)
			}
		})
	}
}

func resolver(title, proposal string) Resolver {
	return Resolver{
		Fetch:   []string{"printf", "%s", title},
		Propose: []string{"printf", "%s", proposal},
	}
}

func TestResolve(t *testing.T) {
	const title = "TICKET-2568: Billing dashboard shows stale totals"

	tests := []struct {
		name     string
		title    string
		proposal string
		want     string
		wantErr  string
	}{
		{
			name:     "a plain answer",
			title:    title,
			proposal: "fix/TICKET-2568-billing-dashboard",
			want:     "fix/TICKET-2568-billing-dashboard",
		},
		{
			// The type is the prompt's business, not checked here.
			name:     "a type outside the original four",
			title:    title,
			proposal: "perf/TICKET-2568-dashboard-lookup",
			want:     "perf/TICKET-2568-dashboard-lookup",
		},
		{
			name:     "an answer with an explanation in front of it",
			title:    title,
			proposal: "Here is a good branch name:\nfix/TICKET-2568-billing-dashboard",
			want:     "fix/TICKET-2568-billing-dashboard",
		},
		{
			name:     "an answer wrapped in whitespace",
			title:    title,
			proposal: "  fix/TICKET-2568-billing-dashboard  \n",
			want:     "fix/TICKET-2568-billing-dashboard",
		},
		{
			name:     "a proposal in no recognisable shape",
			title:    title,
			proposal: "I cannot help with that.",
			wantErr:  "no usable branch name",
		},
		{
			// "Done." is a valid ref name; the key is what separates an answer from a sign-off.
			name:     "a one-word line without the key",
			title:    title,
			proposal: "fix/TICKET-2568-billing-dashboard\nDone.",
			want:     "fix/TICKET-2568-billing-dashboard",
		},
		{
			name:     "a name that forgot the key",
			title:    title,
			proposal: "fix/billing-dashboard",
			wantErr:  "no usable branch name",
		},
		{
			name:     "a name git would refuse",
			title:    title,
			proposal: "fix/TICKET-2568..billing",
			wantErr:  "no usable branch name",
		},
		{
			name:     "a name in backticks",
			title:    title,
			proposal: "`fix/TICKET-2568-billing-dashboard`",
			wantErr:  "no usable branch name",
		},
		{
			name:     "an issue that came back empty",
			title:    "",
			proposal: "fix/TICKET-2568-billing-dashboard",
			wantErr:  "came back empty",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			r := resolver(test.title, test.proposal)
			r.KeyCase = KeyUpper

			got, err := r.Resolve("TICKET-2568")

			if test.wantErr != "" {
				if err == nil {
					t.Fatalf("Resolve = %q, want an error naming %q", got, test.wantErr)
				}
				if !strings.Contains(err.Error(), test.wantErr) {
					t.Fatalf("error = %q, want it to name %q", err, test.wantErr)
				}

				return
			}

			if err != nil {
				t.Fatalf("Resolve: %v", err)
			}
			if got != test.want {
				t.Fatalf("Resolve = %q, want %q", got, test.want)
			}
		})
	}
}

// TestResolveSpellsTheKey pins that the key reaches the commands and the proposal in KeyCase, however it was typed and
// however the model spelt it.
func TestResolveSpellsTheKey(t *testing.T) {
	tests := []struct {
		keyCase  KeyCase
		typed    string
		proposal string
		want     string
	}{
		{"", "AB-1", "fix/AB-1-kafka", "fix/ab-1-kafka"},
		{KeyLower, "AB-1", "fix/Ab-1-kafka", "fix/ab-1-kafka"},
		{KeyLower, "ab-1", "fix/ab-1-kafka", "fix/ab-1-kafka"},
		{KeyUpper, "ab-1", "fix/ab-1-kafka", "fix/AB-1-kafka"},
		{KeyUpper, "AB-1", "fix/AB-1-kafka", "fix/AB-1-kafka"},
	}

	for _, test := range tests {
		t.Run(string(test.keyCase)+"/"+test.typed+"/"+test.proposal, func(t *testing.T) {
			want := test.keyCase.Apply(test.typed)
			r := Resolver{
				Fetch:   []string{"sh", "-c", `[ "$ISSUE_KEY" = "$1" ] && printf 'saw {key}'`, "sh", want},
				Propose: []string{"printf", "%s", test.proposal},
				KeyCase: test.keyCase,
			}

			got, err := r.Resolve(test.typed)
			if err != nil {
				t.Fatalf("Resolve: %v", err)
			}
			if got != test.want {
				t.Fatalf("Resolve = %q, want %q", got, test.want)
			}
		})
	}
}

// TestResolveReportsTheCommand pins that the fetch command's own stderr reaches the error.
func TestResolveReportsTheCommand(t *testing.T) {
	r := Resolver{Fetch: []string{"sh", "-c", "echo 'jira: not configured' >&2; exit 1"}}

	_, err := r.Resolve("TICKET-2568")
	if err == nil {
		t.Fatal("Resolve succeeded with a fetch command that failed")
	}
	if got := err.Error(); !strings.Contains(got, "not configured") {
		t.Fatalf("error = %q, want the command's own complaint", got)
	}
}

// TestResolveHandsTheKeyToFetch pins both routes: the placeholder inside an argument, and the environment.
func TestResolveHandsTheKeyToFetch(t *testing.T) {
	for name, fetch := range map[string][]string{
		"placeholder": {"sh", "-c", `printf 'saw-{key}'`},
		"environment": {"sh", "-c", `printf "saw-$ISSUE_KEY"`},
	} {
		t.Run(name, func(t *testing.T) {
			r := Resolver{
				Fetch:   fetch,
				Propose: []string{"sh", "-c", `printf 'fix/AB-1-%s' "$ISSUE_TITLE"`},
			}

			got, err := r.Resolve("AB-1")
			if err != nil {
				t.Fatalf("Resolve: %v", err)
			}
			if got != "fix/ab-1-saw-ab-1" {
				t.Fatalf("Resolve = %q, want the fetch command to have seen the key", got)
			}
		})
	}
}

// TestResolveRendersThePrompts pins that both prompts arrive rendered, as arguments and as environment.
func TestResolveRendersThePrompts(t *testing.T) {
	r := Resolver{
		Fetch:        []string{"printf", "Kafka retries"},
		SystemPrompt: "system says",
		Prompt:       "name {key} from <{title}>",
		Propose: []string{"sh", "-c", strings.Join([]string{
			`[ "$1" = "system says" ] || exit 1`,
			`[ "$2" = "name ab-1 from <Kafka retries>" ] || exit 2`,
			`[ "$BRANCH_SYSTEM_PROMPT" = "$1" ] || exit 3`,
			`[ "$BRANCH_PROMPT" = "$2" ] || exit 4`,
			`[ "$ISSUE_TITLE" = "Kafka retries" ] || exit 5`,
			`[ -z "$(cat)" ] || exit 6`,
			`printf 'fix/AB-1-kafka-retries'`,
		}, "\n"), "sh", "{system_prompt}", "{prompt}"},
	}

	got, err := r.Resolve("AB-1")
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if got != "fix/ab-1-kafka-retries" {
		t.Fatalf("Resolve = %q", got)
	}
}

// TestDefaultsRender pins that the shipped defaults reference each other's placeholders.
func TestDefaultsRender(t *testing.T) {
	if !strings.Contains(DefaultPrompt, PlaceholderTitle) {
		t.Fatalf("DefaultPrompt does not mention %s", PlaceholderTitle)
	}
	for _, placeholder := range []string{PlaceholderPrompt, PlaceholderSystemPrompt} {
		if !contains(DefaultPropose, placeholder) {
			t.Fatalf("DefaultPropose does not carry %s", placeholder)
		}
	}
	if !contains(DefaultFetch, `"$`+EnvKey+`"`) && !contains(DefaultFetch, PlaceholderKey) {
		t.Fatal("DefaultFetch never receives the key")
	}

	rendered := render(DefaultPrompt, "AB-1", "AB-1: Kafka retries")
	if !strings.HasPrefix(rendered, "Ticket: AB-1: Kafka retries\n") {
		t.Fatalf("rendered prompt = %q", rendered)
	}
}

func contains(argv []string, needle string) bool {
	for _, arg := range argv {
		if strings.Contains(arg, needle) {
			return true
		}
	}

	return false
}
