package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/spf13/cobra"
	"github.com/wavetermdev/waveterm/pkg/vdom"
	"github.com/wavetermdev/waveterm/pkg/vdom/vdomclient"
)

// DictionaryResponse represents the API response structure
type DictionaryResponse []struct {
	Word     string    `json:"word"`
	Phonetic string    `json:"phonetic"`
	Meanings []Meaning `json:"meanings"`
}

type Meaning struct {
	PartOfSpeech string `json:"partOfSpeech"`
	Definitions  []struct {
		Definition string   `json:"definition"`
		Example    string   `json:"example"`
		Synonyms   []string `json:"synonyms"`
	} `json:"definitions"`
}

var wordCmd = &cobra.Command{
	Use:    "word",
	Hidden: true,
	Short:  "show word of the day",
	RunE:   wordRun,
}

func init() {
	rootCmd.AddCommand(wordCmd)
}

func WordStyleTag(ctx context.Context, props map[string]any) any {
	return vdom.Bind(`
    <style>
    .word-container {
        padding: 20px;
        max-width: 600px;
        margin: 0 auto;
        font-family: system-ui, -apple-system, sans-serif;
    }

    .word-title {
        font-size: 32px;
        font-weight: bold;
        color: #2c3e50;
        margin-bottom: 10px;
    }

    .phonetic {
        font-size: 18px;
        color: #666;
        margin-bottom: 20px;
        font-style: italic;
    }

    .part-of-speech {
        font-size: 20px;
        color: #2980b9;
        margin-top: 15px;
        margin-bottom: 10px;
        font-weight: 500;
    }

    .definition {
        margin-bottom: 15px;
        line-height: 1.5;
    }

    .example {
        color: #666;
        font-style: italic;
        margin-top: 5px;
        margin-left: 20px;
    }

    .error {
        color: #e74c3c;
        padding: 20px;
        text-align: center;
        font-size: 18px;
    }

    .loading {
        text-align: center;
        padding: 40px;
        color: #666;
        font-size: 18px;
    }

    .synonyms {
        margin-top: 5px;
        color: #666;
    }

    .synonym-tag {
        display: inline-block;
        background: #f0f0f0;
        padding: 2px 8px;
        border-radius: 4px;
        margin-right: 5px;
        margin-bottom: 5px;
        font-size: 14px;
    }
    </style>
    `, nil)
}
func WordContentTag(ctx context.Context, props map[string]any) any {
	wordData := GlobalVDomClient.GetAtomVal("wordData")
	isLoading := GlobalVDomClient.GetAtomVal("isLoading").(bool)
	errorMsg := GlobalVDomClient.GetAtomVal("errorMsg").(string)

	if isLoading {
		return vdom.Bind(`
        <div className="loading">Loading word of the day...</div>
        `, nil)
	}

	if errorMsg != "" {
		return vdom.Bind(`
        <div className="error"><bindparam key="error"/></div>
        `, map[string]any{
			"error": errorMsg,
		})
	}

	if wordData == nil {
		return vdom.Bind(`
        <div className="loading">Initializing...</div>
        `, nil)
	}

	response := wordData.(DictionaryResponse)
	if len(response) == 0 {
		return vdom.Bind(`
        <div className="error">No word data found</div>
        `, nil)
	}

	word := response[0]

	template := `
    <div className="word-container">
        <div className="word-title"><bindparam key="word"/></div>
        <div className="phonetic"><bindparam key="phonetic"/></div>
    `

	for _, meaning := range word.Meanings {
		template += fmt.Sprintf(`
            <div className="part-of-speech">%s</div>
        `, meaning.PartOfSpeech)

		for _, def := range meaning.Definitions {
			template += fmt.Sprintf(`
                <div className="definition">%s</div>
            `, def.Definition)

			if def.Example != "" {
				template += fmt.Sprintf(`
                    <div className="example">"%s"</div>
                `, def.Example)
			}

			if len(def.Synonyms) > 0 {
				template += `<div className="synonyms">`
				for _, syn := range def.Synonyms[:min(5, len(def.Synonyms))] {
					template += fmt.Sprintf(`
                        <span className="synonym-tag">%s</span>
                    `, syn)
				}
				template += `</div>`
			}
		}
	}

	template += `</div>`

	return vdom.Bind(template, map[string]any{
		"word":     word.Word,
		"phonetic": word.Phonetic,
	})
}

// min returns the minimum of two integers
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// getRandomWord returns a random word to look up
func getRandomWord() string {
	words := []string{
		"serendipity", "ephemeral", "mellifluous", "ineffable", "surreptitious",
		"ethereal", "melancholy", "perspicacious", "sanguine", "vociferous",
		"arduous", "mundane", "pragmatic", "resilient", "tenacious",
	}
	return words[time.Now().Unix()%int64(len(words))]
}

func fetchWordDefinition(word string) (DictionaryResponse, error) {
	url := fmt.Sprintf("https://api.dictionaryapi.dev/api/v2/entries/en/%s", word)
	resp, err := http.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("API returned status code: %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var result DictionaryResponse
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, err
	}

	return result, nil
}

func wordRun(cmd *cobra.Command, args []string) error {
	client, err := vdomclient.MakeClient(&vdom.VDomBackendOpts{CloseOnCtrlC: true})
	if err != nil {
		return err
	}
	GlobalVDomClient = client

	// Register components
	client.RegisterComponent("WordStyleTag", WordStyleTag)
	client.RegisterComponent("WordContentTag", WordContentTag)

	// Initialize state
	client.SetAtomVal("wordData", nil)
	client.SetAtomVal("isLoading", true)
	client.SetAtomVal("errorMsg", "")

	// Set root element
	client.SetRootElem(vdom.Bind(`
    <div>
        <WordStyleTag/>
        <WordContentTag/>
    </div>
    `, nil))

	// Create VDOM context
	err = client.CreateVDomContext(&vdom.VDomTarget{NewBlock: true})
	if err != nil {
		return err
	}

	// Fetch word data
	go func() {
		word := getRandomWord()
		data, err := fetchWordDefinition(word)
		if err != nil {
			client.SetAtomVal("errorMsg", fmt.Sprintf("Error fetching word: %v", err))
		} else {
			client.SetAtomVal("wordData", data)
		}
		client.SetAtomVal("isLoading", false)
		client.SendAsyncInitiation()
	}()

	<-client.DoneCh
	return nil
}
