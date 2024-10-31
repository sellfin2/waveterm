package cmd

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"github.com/wavetermdev/waveterm/pkg/vdom"
	"github.com/wavetermdev/waveterm/pkg/vdom/vdomclient"
)

var lsCmd = &cobra.Command{
	Use:   "ls [directory]",
	Short: "list directory contents with details",
	RunE:  lsRun,
}

func init() {
	rootCmd.AddCommand(lsCmd)
}

type FileInfo struct {
	Name      string
	Size      int64
	Mode      os.FileMode
	ModTime   time.Time
	IsDir     bool
	Extension string
}

func LsStyleTag(ctx context.Context, props map[string]any) any {
	return vdom.Bind(`
    <style>
    .ls-container {
        padding: 20px;
        font-family: monospace;
    }

    .ls-table {
        width: 100%;
        border-collapse: collapse;
    }

    .ls-header {
        text-align: left;
        padding: 10px;
        background: #2c3e50;
        color: white;
        font-weight: bold;
    }

    .ls-row {
        border-bottom: 1px solid #eee;
    }

    .ls-row:hover {
        background: #f5f5f5;
    }

    .ls-cell {
        padding: 8px 10px;
    }

    .dir-name {
        color: #2980b9;
        font-weight: bold;
    }

    .file-name {
        color: #2c3e50;
    }

    .size-cell {
        text-align: right;
        font-family: monospace;
    }

    .time-cell {
        white-space: nowrap;
    }

    .mode-cell {
        font-family: monospace;
        white-space: pre;
    }

    .error {
        color: #e74c3c;
        padding: 20px;
        text-align: center;
        font-size: 16px;
    }
    </style>
    `, nil)
}

// formatSize converts bytes to human readable format
func formatSize(size int64) string {
	if size < 1024 {
		return fmt.Sprintf("%d B", size)
	}
	units := []string{"B", "KB", "MB", "GB", "TB"}
	div := float64(1)
	unitIndex := 0
	for size/int64(div) >= 1024 && unitIndex < len(units)-1 {
		div *= 1024
		unitIndex++
	}
	return fmt.Sprintf("%.1f %s", float64(size)/div, units[unitIndex])
}

// formatMode formats file permissions in a Unix-like style
func formatMode(mode os.FileMode) string {
	output := ""
	if mode.IsDir() {
		output += "d"
	} else {
		output += "-"
	}

	// User permissions
	output += formatPerm(mode, 6)
	// Group permissions
	output += formatPerm(mode, 3)
	// Others permissions
	output += formatPerm(mode, 0)

	return output
}

// formatPerm formats a permission triplet
func formatPerm(mode os.FileMode, shift uint) string {
	output := ""
	output += map[bool]string{true: "r", false: "-"}[(mode>>(shift+2))&1 == 1]
	output += map[bool]string{true: "w", false: "-"}[(mode>>(shift+1))&1 == 1]
	output += map[bool]string{true: "x", false: "-"}[(mode>>shift)&1 == 1]
	return output
}

func LsContentTag(ctx context.Context, props map[string]any) any {
	path := props["path"].(string)

	entries, err := os.ReadDir(path)
	if err != nil {
		return vdom.Bind(`
        <div className="error">Error reading directory: <bindparam key="error"/></div>
        `, map[string]any{
			"error": err.Error(),
		})
	}

	// Convert to FileInfo and sort
	files := make([]FileInfo, 0, len(entries))
	for _, entry := range entries {
		info, err := entry.Info()
		if err != nil {
			continue
		}
		files = append(files, FileInfo{
			Name:      info.Name(),
			Size:      info.Size(),
			Mode:      info.Mode(),
			ModTime:   info.ModTime(),
			IsDir:     info.IsDir(),
			Extension: strings.ToLower(filepath.Ext(info.Name())),
		})
	}

	// Sort: directories first, then by name
	sort.Slice(files, func(i, j int) bool {
		if files[i].IsDir != files[j].IsDir {
			return files[i].IsDir
		}
		return files[i].Name < files[j].Name
	})

	template := `
    <div className="ls-container">
        <table className="ls-table">
            <tr>
                <th className="ls-header">Mode</th>
                <th className="ls-header">Size</th>
                <th className="ls-header">Modified</th>
                <th className="ls-header">Name</th>
            </tr>`

	for _, file := range files {
		nameClass := "file-name"
		if file.IsDir {
			nameClass = "dir-name"
		}

		template += fmt.Sprintf(`
            <tr className="ls-row">
                <td className="ls-cell mode-cell">%s</td>
                <td className="ls-cell size-cell">%s</td>
                <td className="ls-cell time-cell">%s</td>
                <td className="ls-cell %s">%s</td>
            </tr>`,
			formatMode(file.Mode),
			formatSize(file.Size),
			file.ModTime.Format("Jan 02 15:04"),
			nameClass,
			file.Name)
	}

	template += `
        </table>
    </div>`

	return vdom.Bind(template, nil)
}

func lsRun(cmd *cobra.Command, args []string) error {
	path := "."
	if len(args) > 0 {
		path = args[0]
	}

	absPath, err := filepath.Abs(path)
	if err != nil {
		return err
	}

	client, err := vdomclient.MakeClient(&vdom.VDomBackendOpts{CloseOnCtrlC: true})
	if err != nil {
		return err
	}
	GlobalVDomClient = client

	// Register components
	client.RegisterComponent("LsStyleTag", LsStyleTag)
	client.RegisterComponent("LsContentTag", LsContentTag)

	// Set root element
	client.SetRootElem(vdom.Bind(`
    <div>
        <LsStyleTag/>
        <LsContentTag path="#param:path"/>
    </div>
    `, map[string]any{
		"path": absPath,
	}))

	// Create VDOM context
	err = client.CreateVDomContext(&vdom.VDomTarget{NewBlock: true})
	if err != nil {
		return err
	}

	<-client.DoneCh
	return nil
}
