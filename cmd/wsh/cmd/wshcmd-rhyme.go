package cmd

import (
	"context"
	"fmt"
	"log"
	"strings"

	"github.com/sashabaranov/go-openai"
	"github.com/spf13/cobra"
	"github.com/wavetermdev/waveterm/pkg/vdom"
	"github.com/wavetermdev/waveterm/pkg/vdom/vdomclient"
)

const OPENAI_API_KEY = ""

var rhymeCmd = &cobra.Command{
	Use:   "rhyme [word]",
	Short: "find rhyming words using ChatGPT",
	RunE:  rhymeRun,
}

func init() {
	rootCmd.AddCommand(rhymeCmd)
}

type RhymeCategory struct {
	Type  string
	Words []string
}

func RhymeStyleTag(ctx context.Context, props map[string]any) any {
	return vdom.Bind(`
    <style>
    .rhyme-container {
        padding: 20px;
        font-family: system-ui, -apple-system, sans-serif;
        max-width: 800px;
        margin: 0 auto;
    }

    .word-header {
        font-size: 32px;
        font-weight: bold;
        color: #2c3e50;
        margin-bottom: 20px;
        text-align: center;
    }

    .loading {
        text-align: center;
        color: #666;
        padding: 40px;
        font-size: 18px;
    }

    .error {
        color: #e74c3c;
        padding: 20px;
        text-align: center;
        font-size: 18px;
        background: #fdf0ed;
        border-radius: 8px;
        margin: 20px 0;
    }

    .category {
        margin-bottom: 30px;
        background: #ffffff;
        border-radius: 8px;
        padding: 20px;
        box-shadow: 0 2px 4px rgba(0,0,0,0.1);
    }

    .category-title {
        font-size: 20px;
        font-weight: 600;
        color: #2980b9;
        margin-bottom: 15px;
        border-bottom: 2px solid #eee;
        padding-bottom: 8px;
    }

    .word-grid {
        display: grid;
        grid-template-columns: repeat(auto-fill, minmax(120px, 1fr));
        gap: 10px;
    }

    .word-item {
        background: #e8f0fe;
        padding: 8px 12px;
        border-radius: 4px;
        text-align: center;
        transition: all 0.2s;
        color: #2c3e50;
        font-weight: 500;
        border: 1px solid #d0e1fd;
    }

    .word-item:hover {
        background: #d0e1fd;
        transform: translateY(-1px);
    }

    .no-results {
        text-align: center;
        color: #666;
        padding: 40px;
        font-style: italic;
    }
    </style>
    `, nil)
}
func RhymeContentTag(ctx context.Context, props map[string]any) any {
	word := props["word"].(string)
	isLoading := GlobalVDomClient.GetAtomVal("isLoading").(bool)
	errorMsg := GlobalVDomClient.GetAtomVal("errorMsg").(string)
	rhymeData := GlobalVDomClient.GetAtomVal("rhymeData")

	if isLoading {
		return vdom.Bind(`
        <div className="rhyme-container">
            <div className="word-header"><bindparam key="word"/></div>
            <div className="loading">Finding rhymes...</div>
        </div>
        `, map[string]any{
			"word": word,
		})
	}

	if errorMsg != "" {
		return vdom.Bind(`
        <div className="rhyme-container">
            <div className="word-header"><bindparam key="word"/></div>
            <div className="error"><bindparam key="error"/></div>
        </div>
        `, map[string]any{
			"word":  word,
			"error": errorMsg,
		})
	}

	if rhymeData == nil {
		return vdom.Bind(`
        <div className="rhyme-container">
            <div className="word-header"><bindparam key="word"/></div>
            <div className="no-results">No rhymes found</div>
        </div>
        `, map[string]any{
			"word": word,
		})
	}

	categories := rhymeData.([]RhymeCategory)
	if len(categories) == 0 {
		return vdom.Bind(`
        <div className="rhyme-container">
            <div className="word-header"><bindparam key="word"/></div>
            <div className="no-results">No rhymes found</div>
        </div>
        `, map[string]any{
			"word": word,
		})
	}

	template := `
    <div className="rhyme-container">
        <div className="word-header">Rhymes for "<bindparam key="word"/>"</div>`

	for _, category := range categories {
		template += fmt.Sprintf(`
        <div className="category">
            <div className="category-title">%s</div>
            <div className="word-grid">`, category.Type)

		for _, rhyme := range category.Words {
			template += fmt.Sprintf(`
                <div className="word-item">%s</div>`, rhyme)
		}

		template += `
            </div>
        </div>`
	}

	template += `</div>`

	return vdom.Bind(template, map[string]any{
		"word": word,
	})
}

func findRhymes(client *openai.Client, word string) ([]RhymeCategory, error) {
	prompt := fmt.Sprintf(`For the word "%s", provide rhyming words in these categories:
1. Perfect rhymes (same ending sound)
2. Near rhymes (similar ending sound)
3. Family rhymes (same word ending)

Format the response as a simple list with categories and words like this:
Perfect rhymes: word1, word2, word3
Near rhymes: word1, word2, word3
Family rhymes: word1, word2, word3

Only include words that actually exist in English.`, word)

	resp, err := client.CreateChatCompletion(
		context.Background(),
		openai.ChatCompletionRequest{
			Model: openai.GPT3Dot5Turbo,
			Messages: []openai.ChatCompletionMessage{
				{
					Role:    openai.ChatMessageRoleUser,
					Content: prompt,
				},
			},
			Temperature: 0.7,
		},
	)

	if err != nil {
		return nil, err
	}

	// Parse the response
	response := resp.Choices[0].Message.Content
	categories := []RhymeCategory{}

	// Split into lines and parse each category
	lines := strings.Split(response, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		parts := strings.Split(line, ":")
		if len(parts) != 2 {
			continue
		}

		categoryType := strings.TrimSpace(parts[0])
		wordList := strings.Split(strings.TrimSpace(parts[1]), ",")

		// Clean up the words
		words := make([]string, 0)
		for _, word := range wordList {
			word = strings.TrimSpace(word)
			if word != "" {
				words = append(words, word)
			}
		}

		if len(words) > 0 {
			categories = append(categories, RhymeCategory{
				Type:  categoryType,
				Words: words,
			})
		}
	}

	return categories, nil
}

func rhymeRun(cmd *cobra.Command, args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("please provide a word to find rhymes for")
	}
	word := args[0]

	client, err := vdomclient.MakeClient(&vdom.VDomBackendOpts{CloseOnCtrlC: true})
	if err != nil {
		return err
	}
	GlobalVDomClient = client

	// Register components
	client.RegisterComponent("RhymeStyleTag", RhymeStyleTag)
	client.RegisterComponent("RhymeContentTag", RhymeContentTag)

	// Initialize state
	client.SetAtomVal("isLoading", true)
	client.SetAtomVal("errorMsg", "")
	client.SetAtomVal("rhymeData", nil)

	// Set root element
	client.SetRootElem(vdom.Bind(`
    <div>
        <RhymeStyleTag/>
        <RhymeContentTag word="#param:word"/>
    </div>
    `, map[string]any{
		"word": word,
	}))

	// Create VDOM context
	err = client.CreateVDomContext(&vdom.VDomTarget{NewBlock: true})
	if err != nil {
		return err
	}

	// Start fetching rhymes
	go func() {
		openaiClient := openai.NewClient(OPENAI_API_KEY)
		rhymes, err := findRhymes(openaiClient, word)

		if err != nil {
			log.Printf("Error finding rhymes: %v", err)
			client.SetAtomVal("errorMsg", fmt.Sprintf("Error finding rhymes: %v", err))
		} else {
			client.SetAtomVal("rhymeData", rhymes)
		}

		client.SetAtomVal("isLoading", false)
		client.SendAsyncInitiation()
	}()

	<-client.DoneCh
	return nil
}
