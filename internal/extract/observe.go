package extract

import (
	"context"
	"encoding/json"
	"regexp"
	"strings"

	"github.com/thassiov/engram-agent/internal/ollama"
)

// Observation represents a structured observation extracted from a session chunk.
type Observation struct {
	Type     string `json:"type"`
	Title    string `json:"title"`
	Content  string `json:"content"`
	Scope    string `json:"scope"`
	Project  string `json:"project"`
	TopicKey string `json:"topic_key"`
}

// SystemPrompt is the extraction prompt for the LLM.
const SystemPrompt = `You extract structured observations from coding session transcripts. Output ONLY valid JSONL — one JSON object per line. No markdown, no code blocks, no commentary.

IMPORTANT: Be selective. Only extract SIGNIFICANT observations — things worth remembering in future sessions. Skip:
- Questions the user asked (unless the question reveals a preference or constraint)
- Minor actions (checking files, running diagnostic commands)
- Troubleshooting steps that led nowhere
- The user asking for explanations (unless they learned something non-obvious)

Focus on extracting:
- config: A configuration file was created or changed (include what file and what settings)
- decision: A choice was made between alternatives (include what was chosen and why)
- preference: The user stated how they want things done (include the constraint and reasoning)
- discovery: Something non-obvious was learned about the system (include what and why it matters)
- bugfix: A problem was identified and fixed (include root cause and fix)
- architecture: A system design choice was made (include the design and tradeoffs)
- pattern: A convention or approach was established (include the pattern and where it applies)

Each JSON object must have: type, title (short verb-phrase starting with a verb), content (2-4 sentences: what was done/decided, why, what files/systems affected), scope (personal or project), project (system name like portus, engram, grid.local, general), topic_key (lowercase area/subject like portus/va-api, workflow/radio-devices).

Examples:
{"type":"config","title":"Enabled VA-API hardware video decode for Chromium","content":"Created ~/.config/chromium-flags.conf with VaapiVideoDecodeLinuxGL and related flags. Offloads video decode from CPU to Intel Iris Xe GPU.","scope":"project","project":"portus","topic_key":"portus/chromium-vaapi"}
{"type":"preference","title":"Never disable WiFi or Bluetooth via power management","content":"User has active SSH sessions over WiFi and uses Bluetooth headphones and mouse. TLP radio device wizard rules must remain unconfigured.","scope":"personal","project":"general","topic_key":"workflow/radio-devices"}`

// jsonObjectRe matches top-level JSON objects in text.
var jsonObjectRe = regexp.MustCompile(`\{[^{}]*\}`)

// ExtractObservations sends a chunk to ollama and parses the resulting observations.
func ExtractObservations(ctx context.Context, client *ollama.Client, chunk Chunk) ([]Observation, error) {
	prompt := "Extract observations from this conversation excerpt:\n\n" + chunk.Text

	response, err := client.Chat(ctx, SystemPrompt, prompt)
	if err != nil {
		return nil, err
	}

	return parseObservations(response), nil
}

// parseObservations extracts valid observations from LLM output.
// Handles messy output by finding JSON objects via regex.
func parseObservations(text string) []Observation {
	matches := jsonObjectRe.FindAllString(strings.ReplaceAll(text, "\n", " "), -1)

	var observations []Observation
	for _, m := range matches {
		var obs Observation
		if err := json.Unmarshal([]byte(m), &obs); err != nil {
			continue
		}
		if obs.Type == "" || obs.Title == "" {
			continue
		}
		if obs.Scope == "" {
			obs.Scope = "project"
		}
		if obs.Project == "" {
			obs.Project = "general"
		}
		observations = append(observations, obs)
	}
	return observations
}
