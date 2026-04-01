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

STRICT RULES:
- DO NOT expand acronyms or abbreviations. Write them exactly as they appear (OCI, LXC, PG, PM, TLP, etc.)
- DO NOT guess what terms mean. If unsure, use the term verbatim from the conversation.
- DO NOT invent details not present in the text. Only describe what actually happened.
- DO NOT add explanations or context the user did not provide.
- Use correct spelling for all names and tools mentioned in the conversation.

Be selective. Only extract SIGNIFICANT observations worth remembering in future sessions. Skip:
- Questions without conclusions
- Minor actions (checking files, running diagnostic commands)
- Troubleshooting steps that led nowhere

Observation types:
- config: A configuration file was created or changed
- decision: A choice was made between alternatives (include what and why)
- preference: The user stated how they want things done
- discovery: Something non-obvious was learned
- bugfix: A problem was identified and fixed (include root cause and fix)
- architecture: A system design choice was made
- pattern: A convention or approach was established

Each JSON object must have these fields:
- type: one of the types above
- title: short verb-phrase (start with a verb, max 10 words)
- content: 2-3 sentences describing what was done/decided and why
- scope: "personal" or "project"
- project: system name exactly as mentioned (e.g. portus, engram, grid.local, general)
- topic_key: lowercase area/subject (e.g. portus/va-api, engram/sync)

Examples:
{"type":"config","title":"Enabled VA-API hardware video decode for Chromium","content":"Created ~/.config/chromium-flags.conf with VaapiVideoDecodeLinuxGL flags. Offloads video decode from CPU to Iris Xe GPU.","scope":"project","project":"portus","topic_key":"portus/chromium-vaapi"}
{"type":"decision","title":"Chose TLP drop-in config over editing main file","content":"Created /etc/tlp.d/01-portus.conf instead of modifying /etc/tlp.conf. Keeps main config stock so pacman upgrades don't overwrite settings.","scope":"project","project":"portus","topic_key":"portus/tlp-config"}`

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
