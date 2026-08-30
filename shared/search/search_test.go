package search

import "testing"

func TestMatch(t *testing.T) {
	tests := []struct {
		name  string
		text  string
		query string
		want  bool
	}{
		{"empty query matches", "anything", "", true},
		{"substring", "lorem/dolor-amet", "or-am", true},
		{"subsequence fails", "lorem/dolor-amet", "dolamet", false},
		{"term does not skip a delimiter", "chore/kafka-test", "chorekafka", false},
		{"digits must be contiguous", "fix/PROJ-2481-kafka-v1", "2811", false},
		{"delimiter in the term", "chore/kafka-test", "chore/kafka", true},
		{"case insensitive", "PROJ-2478", "proj", true},
		{"terms match in any order", "lorem/ipsum feature/PROJ-2478", "2478 ipsum", true},
		{"every term must match", "lorem/ipsum feature/PROJ-2478", "2478 dolor", false},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if _, ok := Match(test.text, test.query); ok != test.want {
				t.Fatalf("Match(%q, %q) = %v, want %v", test.text, test.query, ok, test.want)
			}
		})
	}
}

// TestMatchRanking pins that a term on word boundaries beats one inside a word.
func TestMatchRanking(t *testing.T) {
	tests := []struct {
		name   string
		query  string
		better string
		worse  string
	}{
		{"word start beats mid-word", "dolor", "lorem/dolor", "condolor"},
		{"whole word beats a word prefix", "2811", "fix/PROJ-2811-amet", "fix/PROJ-28110-amet"},
		{"best occurrence counts", "dolor", "condolores-dolor", "lorem-condolores"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			better, ok := Match(test.better, test.query)
			if !ok {
				t.Fatalf("Match(%q, %q) did not match", test.better, test.query)
			}
			worse, ok := Match(test.worse, test.query)
			if !ok {
				t.Fatalf("Match(%q, %q) did not match", test.worse, test.query)
			}
			if better.Score <= worse.Score {
				t.Fatalf("score(%q)=%d should beat score(%q)=%d",
					test.better, better.Score, test.worse, worse.Score)
			}
		})
	}
}

// TestMatchPositions pins that positions mark the best occurrence, not the first.
func TestMatchPositions(t *testing.T) {
	result, ok := Match("condolo-dolo", "dolo")
	if !ok {
		t.Fatal("expected a match")
	}
	want := []int{8, 9, 10, 11}
	if len(result.Positions) != len(want) {
		t.Fatalf("positions = %v, want %v", result.Positions, want)
	}
	for i, position := range want {
		if result.Positions[i] != position {
			t.Fatalf("positions = %v, want %v", result.Positions, want)
		}
	}
}
