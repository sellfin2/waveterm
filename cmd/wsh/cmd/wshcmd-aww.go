package cmd

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"

	"image"
	_ "image/gif"
	"image/jpeg"
	_ "image/png"

	"github.com/nfnt/resize"
	"github.com/spf13/cobra"
	"github.com/wavetermdev/waveterm/pkg/vdom"
	"github.com/wavetermdev/waveterm/pkg/vdom/vdomclient"
)

type RedditListing struct {
	Data struct {
		Children []struct {
			Data struct {
				Title     string `json:"title"`
				URL       string `json:"url"`
				Permalink string `json:"permalink"`
			} `json:"data"`
		} `json:"children"`
	} `json:"data"`
}

var postPosition int

var awwCmd = &cobra.Command{
	Use:   "aww [position]",
	Short: "show today's top r/aww image",
	RunE:  awwRun,
}

func init() {
	awwCmd.Flags().IntVarP(&postPosition, "position", "p", 0, "position of post (0-based)")
	rootCmd.AddCommand(awwCmd)
}

func AwwStyleTag(ctx context.Context, props map[string]any) any {
	return vdom.Bind(`
    <style>
    .aww-container {
        padding: 20px;
        max-width: 800px;
        margin: 0 auto;
        text-align: center;
        font-family: system-ui, -apple-system, sans-serif;
    }

    .title {
        font-size: 24px;
        color: #2c3e50;
        margin-bottom: 20px;
        line-height: 1.4;
    }

    .image-container {
        margin: 20px 0;
        display: inline-block;
        max-width: 100%;
        background: white;
        padding: 10px;
        border-radius: 8px;
        box-shadow: 0 2px 4px rgba(0,0,0,0.1);
    }

    .image-container img {
        max-width: 100%;
        height: auto;
        border-radius: 4px;
    }

    .link {
        display: inline-block;
        margin-top: 15px;
        color: #2980b9;
        text-decoration: none;
        font-size: 16px;
    }

    .link:hover {
        text-decoration: underline;
    }

    .loading {
        padding: 40px;
        text-align: center;
        color: #666;
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
    </style>
    `, nil)
}

func AwwContentTag(ctx context.Context, props map[string]any) any {
	isLoading := GlobalVDomClient.GetAtomVal("isLoading").(bool)
	errorMsg := GlobalVDomClient.GetAtomVal("errorMsg").(string)
	imageData := GlobalVDomClient.GetAtomVal("imageData")
	title := GlobalVDomClient.GetAtomVal("title")
	postURL := GlobalVDomClient.GetAtomVal("postURL")

	if isLoading {
		return vdom.Bind(`
        <div className="aww-container">
            <div className="loading">Finding today's cutest image...</div>
        </div>
        `, nil)
	}

	if errorMsg != "" {
		return vdom.Bind(`
        <div className="aww-container">
            <div className="error"><bindparam key="error"/></div>
        </div>
        `, map[string]any{
			"error": errorMsg,
		})
	}

	return vdom.Bind(`
    <div className="aww-container">
        <div className="title"><bindparam key="title"/></div>
        <a href="#param:postURL" className="image-container">
            <img src="#param:imageData" alt="Cute animal of the day"/>
        </a>
        <a href="#param:postURL" className="link">View on Reddit</a>
    </div>
    `, map[string]any{
		"title":     title,
		"imageData": imageData,
		"postURL":   postURL,
	})
}

// downloadImage downloads an image from URL and returns its bytes
func downloadImage(url string) ([]byte, error) {
	resp, err := http.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("bad status: %s", resp.Status)
	}

	return io.ReadAll(resp.Body)
}

// resizeImage resizes the image to be under 64KB
func resizeImage(imgBytes []byte) (string, error) {
	// Decode image
	img, _, err := image.Decode(strings.NewReader(string(imgBytes)))
	if err != nil {
		return "", err
	}

	// Start with original size
	width := uint(img.Bounds().Dx())
	height := uint(img.Bounds().Dy())
	scaleFactor := 1.0

	// Keep trying until we get under 64KB
	for {
		// Resize image
		newWidth := uint(float64(width) * scaleFactor)
		newHeight := uint(float64(height) * scaleFactor)
		resized := resize.Resize(newWidth, newHeight, img, resize.Lanczos3)

		// Encode to JPEG with quality 85
		var buf strings.Builder
		jpeg.Encode(base64.NewEncoder(base64.StdEncoding, &buf), resized, &jpeg.Options{Quality: 85})
		encoded := buf.String()

		// Check if we're under 64KB
		if len(encoded) < 64*1024 {
			return fmt.Sprintf("data:image/jpeg;base64,%s", encoded), nil
		}

		// Reduce size by 10% and try again
		scaleFactor *= 0.9
	}
}

func fetchTopAwwPost(position int) (string, string, string, error) {
	// Get a few extra posts in case some aren't images
	log.Printf("Fetching top %d posts from r/aww, pos:%d\n", position+5, position)
	limit := position + 5
	resp, err := http.Get(fmt.Sprintf("https://www.reddit.com/r/aww/top.json?t=day&limit=%d", limit))
	if err != nil {
		return "", "", "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", "", "", fmt.Errorf("bad status: %s", resp.Status)
	}

	var listing RedditListing
	if err := json.NewDecoder(resp.Body).Decode(&listing); err != nil {
		return "", "", "", err
	}
	log.Printf("got listings num:%d\n", len(listing.Data.Children))

	if len(listing.Data.Children) <= position {
		return "", "", "", fmt.Errorf("no post found at position %d", position)
	}

	post := listing.Data.Children[position].Data
	return post.Title, post.URL, "https://reddit.com" + post.Permalink, nil
}

func awwRun(cmd *cobra.Command, args []string) error {
	if postPosition < 0 {
		return fmt.Errorf("position must be non-negative")
	}

	client, err := vdomclient.MakeClient(&vdom.VDomBackendOpts{CloseOnCtrlC: true})
	if err != nil {
		return err
	}
	GlobalVDomClient = client

	// Register components
	client.RegisterComponent("AwwStyleTag", AwwStyleTag)
	client.RegisterComponent("AwwContentTag", AwwContentTag)

	// Initialize state
	client.SetAtomVal("isLoading", true)
	client.SetAtomVal("errorMsg", "")
	client.SetAtomVal("imageData", "")
	client.SetAtomVal("title", "")
	client.SetAtomVal("postURL", "")

	// Set root element
	client.SetRootElem(vdom.Bind(`
    <div>
        <AwwStyleTag/>
        <AwwContentTag/>
    </div>
    `, nil))

	// Create VDOM context
	err = client.CreateVDomContext(&vdom.VDomTarget{NewBlock: true})
	if err != nil {
		return err
	}

	// Start fetching the image
	go func() {
		// Fetch top post
		title, imageURL, postURL, err := fetchTopAwwPost(postPosition)
		if err != nil {
			client.SetAtomVal("errorMsg", fmt.Sprintf("Error fetching post: %v", err))
			client.SetAtomVal("isLoading", false)
			client.SendAsyncInitiation()
			return
		}

		// Download image
		imgBytes, err := downloadImage(imageURL)
		if err != nil {
			client.SetAtomVal("errorMsg", fmt.Sprintf("Error downloading image: %v", err))
			client.SetAtomVal("isLoading", false)
			client.SendAsyncInitiation()
			return
		}

		// Resize and encode image
		base64Image, err := resizeImage(imgBytes)
		if err != nil {
			client.SetAtomVal("errorMsg", fmt.Sprintf("Error processing image: %v", err))
			client.SetAtomVal("isLoading", false)
			client.SendAsyncInitiation()
			return
		}

		// Update state with results
		client.SetAtomVal("title", title)
		client.SetAtomVal("imageData", base64Image)
		client.SetAtomVal("postURL", postURL)
		client.SetAtomVal("isLoading", false)
		client.SendAsyncInitiation()
	}()

	<-client.DoneCh
	return nil
}
