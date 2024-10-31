package cmd

import (
	"context"
	"log"
	"math/rand"
	"strconv"
	"time"

	"github.com/spf13/cobra"
	"github.com/wavetermdev/waveterm/pkg/vdom"
	"github.com/wavetermdev/waveterm/pkg/vdom/vdomclient"
)

var moleCmd = &cobra.Command{
	Use:    "mole",
	Hidden: true,
	Short:  "launch whack-a-mole game",
	RunE:   moleRun,
}

func init() {
	rootCmd.AddCommand(moleCmd)
}
func MoleCellTag(ctx context.Context, props map[string]any) any {
	index := props["index"].(int)
	molePosition := GlobalVDomClient.GetAtomVal("molePosition").(int)

	className := "cell"
	showMole := false
	if molePosition == index {
		className += " mole"
		showMole = true
	}

	template := `
    <button className="#param:className" onClick="#param:clickHandler">`

	if showMole {
		// This is our SVG converted to base64
		template += `<img src="data:image/svg+xml;base64,PHN2ZyB4bWxucz0iaHR0cDovL3d3dy53My5vcmcvMjAwMC9zdmciIHZpZXdCb3g9IjAgMCAxMDAgMTAwIj48Y2lyY2xlIGN4PSI1MCIgY3k9IjUwIiByPSIzNSIgZmlsbD0iIzhCNDUxMyIvPjxlbGxpcHNlIGN4PSI1MCIgY3k9IjYwIiByeD0iMjUiIHJ5PSIyMCIgZmlsbD0iI0RFQjg4NyIvPjxlbGxpcHNlIGN4PSI1MCIgY3k9IjQ1IiByeD0iMTIiIHJ5PSI4IiBmaWxsPSIjNDYzMjIyIi8+PGNpcmNsZSBjeD0iMzUiIGN5PSIzNSIgcj0iNSIgZmlsbD0iYmxhY2siLz48Y2lyY2xlIGN4PSI2NSIgY3k9IjM1IiByPSI1IiBmaWxsPSJibGFjayIvPjxjaXJjbGUgY3g9IjMzIiBjeT0iMzMiIHI9IjIiIGZpbGw9IndoaXRlIi8+PGNpcmNsZSBjeD0iNjMiIGN5PSIzMyIgcj0iMiIgZmlsbD0id2hpdGUiLz48bGluZSB4MT0iMzUiIHkxPSI0NSIgeDI9IjIwIiB5Mj0iNDAiIHN0cm9rZT0iYmxhY2siIHN0cm9rZS13aWR0aD0iMiIvPjxsaW5lIHgxPSIzNSIgeTE9IjQ4IiB4Mj0iMjAiIHkyPSI0OCIgc3Ryb2tlPSJibGFjayIgc3Ryb2tlLXdpZHRoPSIyIi8+PGxpbmUgeDE9IjM1IiB5MT0iNTEiIHgyPSIyMCIgeTI9IjU2IiBzdHJva2U9ImJsYWNrIiBzdHJva2Utd2lkdGg9IjIiLz48bGluZSB4MT0iNjUiIHkxPSI0NSIgeDI9IjgwIiB5Mj0iNDAiIHN0cm9rZT0iYmxhY2siIHN0cm9rZS13aWR0aD0iMiIvPjxsaW5lIHgxPSI2NSIgeTE9IjQ4IiB4Mj0iODAiIHkyPSI0OCIgc3Ryb2tlPSJibGFjayIgc3Ryb2tlLXdpZHRoPSIyIi8+PGxpbmUgeDE9IjY1IiB5MT0iNTEiIHgyPSI4MCIgeTI9IjU2IiBzdHJva2U9ImJsYWNrIiBzdHJva2Utd2lkdGg9IjIiLz48L3N2Zz4=" width="60" height="60" alt="mole"></img>`
	}

	template += `</button>`

	return vdom.Bind(template, map[string]any{
		"className":    className,
		"clickHandler": props["onCellClick"],
	})
}

func MoleStyleTag(ctx context.Context, props map[string]any) any {
	return vdom.Bind(`
    <style>
    .game-container {
        padding: 20px;
        text-align: center;
    }

    .score {
        font-size: 24px;
        margin-bottom: 20px;
    }

    .grid {
        display: inline-grid;
        grid-template-columns: repeat(3, 100px);
        grid-gap: 10px;
    }

    .cell {
        width: 100px;
        height: 100px;
        border: 2px solid #333;
        border-radius: 8px;
        cursor: pointer;
        transition: all 0.2s;
        display: flex;
        align-items: center;
        justify-content: center;
        background: #f0f0f0;
        padding: 0;
    }

    .cell.mole {
        background-color: #8a6343;
    }

    .cell:hover {
        opacity: 0.9;
        transform: scale(0.95);
    }

    .cell img {
        animation: pop-up 0.3s ease-out;
    }

    @keyframes pop-up {
        0% { transform: translateY(50px) scale(0.5); opacity: 0; }
        100% { transform: translateY(0) scale(1); opacity: 1; }
    }

    .start-button {
        margin-top: 20px;
        padding: 10px 20px;
        font-size: 18px;
        border-radius: 4px;
        border: none;
        background: #2196F3;
        color: white;
        cursor: pointer;
    }

    .start-button:hover {
        background: #1976D2;
    }
    </style>
    `, nil)
}

var moleScore int = 0

func MoleGameTag(ctx context.Context, props map[string]any) any {
	makeHandleCellClick := func(index int) func() {
		return func() {
			currentScore := GlobalVDomClient.GetAtomVal("moleScore").(int)
			molePosition := GlobalVDomClient.GetAtomVal("molePosition").(int)
			isActive := GlobalVDomClient.GetAtomVal("moleGameActive").(bool)

			log.Printf("cell clicked: %d (active:%v)\n", index, isActive)

			if !isActive {
				return
			}

			// Check if we clicked the right mole
			if molePosition == index {
				GlobalVDomClient.SetAtomVal("moleScore", currentScore+1)
				moleScore++

				// Move mole to new random position
				newPosition := rand.Intn(9)
				GlobalVDomClient.SetAtomVal("molePosition", newPosition)
			}
		}
	}

	toggleGame := func() {
		isActive := GlobalVDomClient.GetAtomVal("moleGameActive").(bool)
		if isActive {
			GlobalVDomClient.SetAtomVal("moleGameActive", false)
			GlobalVDomClient.SetAtomVal("moleScore", 0)
			moleScore = 0
			GlobalVDomClient.SetAtomVal("molePosition", -1)
		} else {
			GlobalVDomClient.SetAtomVal("moleGameActive", true)
			GlobalVDomClient.SetAtomVal("molePosition", rand.Intn(9))
		}
	}

	isActive := GlobalVDomClient.GetAtomVal("moleGameActive").(bool)
	buttonText := "Start Game"
	if isActive {
		buttonText = "Stop Game"
	}

	return vdom.Bind(`
    <div className="game-container">
        <MoleStyleTag/>
        <div className="score">Score: <bindparam key="moleScore"/></div>
        <div className="grid">
            <MoleCellTag index="#param:index0" onCellClick="#param:click0"/>
            <MoleCellTag index="#param:index1" onCellClick="#param:click1"/>
            <MoleCellTag index="#param:index2" onCellClick="#param:click2"/>
            <MoleCellTag index="#param:index3" onCellClick="#param:click3"/>
            <MoleCellTag index="#param:index4" onCellClick="#param:click4"/>
            <MoleCellTag index="#param:index5" onCellClick="#param:click5"/>
            <MoleCellTag index="#param:index6" onCellClick="#param:click6"/>
            <MoleCellTag index="#param:index7" onCellClick="#param:click7"/>
            <MoleCellTag index="#param:index8" onCellClick="#param:click8"/>
        </div>
        <button className="start-button" onClick="#param:toggleGame">
            <bindparam key="buttonText"/>
        </button>
    </div>
    `, map[string]any{
		"toggleGame": toggleGame,
		"buttonText": buttonText,
		"index0":     0,
		"index1":     1,
		"index2":     2,
		"index3":     3,
		"index4":     4,
		"index5":     5,
		"index6":     6,
		"index7":     7,
		"index8":     8,
		"click0":     makeHandleCellClick(0),
		"click1":     makeHandleCellClick(1),
		"click2":     makeHandleCellClick(2),
		"click3":     makeHandleCellClick(3),
		"click4":     makeHandleCellClick(4),
		"click5":     makeHandleCellClick(5),
		"click6":     makeHandleCellClick(6),
		"click7":     makeHandleCellClick(7),
		"click8":     makeHandleCellClick(8),
		"moleScore":  strconv.Itoa(moleScore),
	})
}

func moleRun(cmd *cobra.Command, args []string) error {
	client, err := vdomclient.MakeClient(&vdom.VDomBackendOpts{CloseOnCtrlC: true})
	if err != nil {
		return err
	}
	GlobalVDomClient = client

	// Initialize random seed
	rand.Seed(time.Now().UnixNano())

	// Register components
	client.RegisterComponent("MoleStyleTag", MoleStyleTag)
	client.RegisterComponent("MoleCellTag", MoleCellTag)
	client.RegisterComponent("MoleGameTag", MoleGameTag)

	// Initialize state
	client.SetAtomVal("moleScore", 0)
	client.SetAtomVal("molePosition", -1)
	client.SetAtomVal("moleGameActive", false)

	// Set root element
	client.SetRootElem(vdom.Bind(`<MoleGameTag/>`, nil))

	// Create VDOM context
	err = client.CreateVDomContext(&vdom.VDomTarget{NewBlock: true})
	if err != nil {
		return err
	}

	// Start game loop when active
	go func() {
		for {
			time.Sleep(1 * time.Second)
			if client.GetAtomVal("moleGameActive").(bool) {
				newPos := rand.Intn(9)
				client.SetAtomVal("molePosition", newPos)
				log.Printf("new mole position: %d\n", newPos)
				client.SendAsyncInitiation()
			}
		}
	}()

	<-client.DoneCh
	return nil
}
