package main

import (
	"flag"
	"fmt"
	"log"

	"github.com/jroimartin/gocui"
	"io/ioutil"
	// "math"
	"bytes"
	"github.com/go-git/go-git/v5"
	"github.com/jedib0t/go-pretty/v6/table"
	"math/rand"
	// 	"github.com/jedib0t/go-pretty/v6/text"
	"errors"
	"github.com/fatih/color"
	"github.com/xeonx/timeago"
	"os"
	"os/exec"
	"path"
	"regexp"
	"strings"
	"time"
)

type RepoInfo struct {
	path                 string
	branchName           string
	latestHash           string
	time                 time.Time
	email                string
	message              string
	isDirty              bool
	statusMessagePointer *string
}

// https://github.com/go-git/go-git/blob/_examples/common.go#L19
func CheckIfError(err error) {
	if err == nil {
		return
	}

	fmt.Printf("\x1b[31;1m%s\x1b[0m\n", fmt.Sprintf("error: %s", err))
	os.Exit(1)
}

func FindRepos(startDirectory string, out *[]string, maxDepth int) {

	pattern := regexp.MustCompile("^\\.(?P<type>git)$")

	files, err := ioutil.ReadDir(startDirectory)
	if err != nil {
		log.Println(err)
		return
	}

	for _, fileInfo := range files {
		fullPath := path.Join(startDirectory, fileInfo.Name())

		maybeMatches := pattern.FindStringSubmatch(fileInfo.Name())
		if len(maybeMatches) > 1 {
			fmt.Printf("found a [%s] repo: %s\n", maybeMatches[1], fullPath)
			*out = append(*out, startDirectory)
		} else if fileInfo.IsDir() && maxDepth > 1 {
			// fmt.Printf("NEXT %s\n", fullPath)
			FindRepos(fullPath, out, maxDepth-1)
		}
	}
}

func renderBranchName(ri *RepoInfo) string {
	branchNameSplit := strings.Split(ri.branchName, "/")
	return branchNameSplit[len(branchNameSplit)-1]
}

func renderTime(ri *RepoInfo) string {
	return timeago.English.Format(ri.time)
}

func renderCommitMessage(ri *RepoInfo) string {
	mainMessage := strings.Split(ri.message, "\n")[0]
	maxLen := 40
	if len(mainMessage) < maxLen-3 {
		return mainMessage
	} else {
		return fmt.Sprintf("%s...", mainMessage[:maxLen])
	}
}

func renderAuthorString(ri *RepoInfo) string {
	var authorString string
	emailSplit := strings.Split(ri.email, "@")
	if len(emailSplit) == 1 {
		authorString = ri.email
	} else {
		domainSplit := strings.Split(emailSplit[1], ".")
		if len(domainSplit) == 1 {
			authorString = emailSplit[1]
		} else {
			authorString = fmt.Sprintf("%s %s",
				emailSplit[0],
				domainSplit[len(domainSplit)-2])
		}
	}
	return authorString
}

func renderRepoInfo(ri *RepoInfo) string {
	return fmt.Sprintf("%s\t%s\t%s\t%s\t%s\t%s\n",
		ri.path,
		renderBranchName(ri),
		ri.latestHash[:7],
		renderTime(ri),
		renderAuthorString(ri),
		renderCommitMessage(ri),
	)
}

func renderRepoInfoTable(repoInfos *[]RepoInfo, tableStyle table.Style) string {
	if false {
		buffer := new(bytes.Buffer)
		for _, ri := range *repoInfos {
			buffer.WriteString(renderRepoInfo(&ri))
		}
		return buffer.String()
	} else {
		tableOut := table.NewWriter()
		tableOut.AppendHeader(
			table.Row{"#", "dirty?", "path", "branch", "hash", "time", "email", "message", "status"})

		for i, ri := range *repoInfos {
			var modifiedMarker string
			if ri.isDirty {
				modifiedMarker = "X"
			} else {
				modifiedMarker = " "
			}

			// this section might be inducing sigsegv
			var statusMessage string
			if ri.statusMessagePointer != nil {
				statusMessage = *ri.statusMessagePointer
			} else {
				statusMessage = "..."
			}
			tableOut.AppendRow(
				table.Row{
					// i + 1,
					fmt.Sprintf("%s/%p", i, &ri),
					modifiedMarker, ri.path, renderBranchName(&ri), ri.latestHash[:7], renderTime(&ri), renderAuthorString(&ri),
					renderCommitMessage(&ri),
					statusMessage,
				})
		}
		tableOut.SetStyle(tableStyle)
		return tableOut.Render()
	}
}

func renderRepoInfoTableColored(repoInfos *[]RepoInfo) string {
	return renderRepoInfoTable(repoInfos, table.StyleColoredBright)
}

func renderRepoInfoTablePlain(repoInfos *[]RepoInfo) string {
	tableStyle := table.Style{
		Name:    "Plain",
		Box:     table.StyleBoxDefault,
		Color:   table.ColorOptionsDefault,
		Format:  table.FormatOptionsDefault,
		HTML:    table.DefaultHTMLOptions,
		Options: table.OptionsNoBordersAndSeparators,
		Title:   table.TitleOptionsDefault,
	}
	return renderRepoInfoTable(repoInfos, tableStyle)
}

// https://gist.githubusercontent.com/jroimartin/b78c4c33c67a289dc028dd7d562e1f6e/raw/1981c7565960d85298598b78745afaaf4d19b704/goroutine_widget.go
type StatusbarWidget struct {
	name string
	x, y int
	w    int
	val  float64
}

func NewStatusbarWidget(name string, x, y, w int) *StatusbarWidget {
	return &StatusbarWidget{name: name, x: x, y: y, w: w}
}

func (w *StatusbarWidget) SetVal(val float64) error {
	if val < 0 || val > 1 {
		return errors.New("invalid value")
	}
	w.val = val
	return nil
}

func (w *StatusbarWidget) Val() float64 {
	return w.val
}

func runCommand(args ...string) string {
	buf := &bytes.Buffer{}
	cmd := exec.Command(args[0], args[1:]...)
	cmd.Stdout = buf
	if err := cmd.Start(); err != nil {
		log.Printf("failed to start command")
	}
	if err := cmd.Wait(); err != nil {
		log.Printf("command failed")
	}
	return buf.String()
}

func update(g *gocui.Gui, sw *StatusbarWidget) {
	for {
		g.Update(func(g *gocui.Gui) error {
			v, err := g.SetView("mything", 1, 20, 80, 30)
			if err != nil {
				// handle error
			}
			v.Clear()

			commandOutput1 := runCommand("bash", "-c", "ps ax | tail -1")
			commandOutput2 := runCommand("date")
			fmt.Fprintln(v, fmt.Sprintf("%s\n-> %s\n-> %s", time.Now(), commandOutput1, commandOutput2))
			return nil
		})
		time.Sleep(2000 * time.Millisecond)
	}
}

func renderCuiRepoInfoTable(v *gocui.View, repoInfos *[]RepoInfo) {
	tableString := renderRepoInfoTablePlain(repoInfos)
	red := color.New(color.FgRed).SprintFunc()

	tableStringSplit := strings.Split(tableString, "\n")
	for i := 0; i < len(tableStringSplit); i++ {
		line := tableStringSplit[i]

		if i == 0 {
			fmt.Fprintln(v, line)
		} else {
			repoInfo := (*repoInfos)[i-1]
			var coloredLine string
			if repoInfo.isDirty {
				coloredLine = red(line)
			} else {
				coloredLine = line
			}
			fmt.Fprintf(v, coloredLine+"\n")
		}
	}
}

func updateTable(g *gocui.Gui, repoInfos *[]RepoInfo) error {
	for {
		g.Update(func(g *gocui.Gui) error {
			v1, err := g.View("mything")
			if err != nil {
				// handle error
			}
			v1.Clear()
			fmt.Fprintln(v1, fmt.Sprintf("%p", repoInfos))

			v, err := g.View("repoList")
			if err != nil {
				// handle error
			}
			v.Clear()

			for i := 0; i < len(*repoInfos); i++ {
				myString := fmt.Sprintf("%d", rand.Intn(100))
				(*repoInfos)[i].statusMessagePointer = &myString
			}
			renderCuiRepoInfoTable(v, repoInfos)
			return nil
		})
		time.Sleep(3000 * time.Millisecond)
	}
}

func makeLayout(repoInfos *[]RepoInfo) func(g *gocui.Gui) error {
	return func(g *gocui.Gui) error {
		maxX, maxY := g.Size()
		if v, err := g.SetView("repoList", -1, -1, maxX, maxY); err != nil {
			if err != gocui.ErrUnknownView {
				return err
			}
			v.Highlight = true
			v.SelBgColor = gocui.ColorGreen
			v.SelFgColor = gocui.ColorBlack

			if err := v.SetCursor(0, 1); err != nil {
				return err
			}

			renderCuiRepoInfoTable(v, repoInfos)
		}
		if _, err := g.SetCurrentView("repoList"); err != nil {
			return err
		}
		return nil
	}
}

func cursorDown(g *gocui.Gui, v *gocui.View) error {
	if v != nil {
		cx, cy := v.Cursor()
		if err := v.SetCursor(cx, cy+1); err != nil {
			ox, oy := v.Origin()
			if err := v.SetOrigin(ox, oy+1); err != nil {
				return err
			}
		}
	}
	return nil
}

func cursorUp(g *gocui.Gui, v *gocui.View) error {
	if v != nil {
		ox, oy := v.Origin()
		cx, cy := v.Cursor()
		if err := v.SetCursor(cx, cy-1); err != nil && oy > 0 {
			if err := v.SetOrigin(ox, oy-1); err != nil {
				return err
			}
		}
	}
	return nil
}

func makeCursorReader(repoInfos *[]RepoInfo, direction int) func(g *gocui.Gui, v *gocui.View) error {
	var step int
	if direction > 0 {
		step = 1
	} else {
		step = -1
	}
	maxY := len(*repoInfos)
	return func(g *gocui.Gui, v *gocui.View) error {
		if v != nil {
			ox, oy := v.Origin()
			cx, cy := v.Cursor()
			nextCy := cy + step
			if 0 < nextCy && nextCy <= maxY {
				if err := v.SetCursor(cx, cy+step); err != nil {
					if err := v.SetOrigin(ox, oy+step); err != nil {
						return err
					}
				}
			}
		}
		return nil
	}
}

func hideInfoModal(g *gocui.Gui, v *gocui.View) error {
	if err := g.DeleteView("infoModal"); err != nil {
		return err
	}
	if _, err := g.SetCurrentView("repoList"); err != nil {
		return err
	}
	return nil
}

func showInfoModal(g *gocui.Gui, v *gocui.View) error {
	var l string
	var err error

	_, cy := v.Cursor()
	if l, err = v.Line(cy); err != nil {
		l = ""
	}

	_, oy := v.Origin()
	maxX, maxY := g.Size()
	if v, err := g.SetView("infoModal", maxX/2-30, maxY/2, maxX/2+30, maxY/2+2); err != nil {
		if err != gocui.ErrUnknownView {
			return err
		}
		l = fmt.Sprintf("C: o %v,c %v %s", oy, cy, l)
		fmt.Fprintln(v, l)
		if _, err := g.SetCurrentView("infoModal"); err != nil {
			return err
		}
	}
	return nil
}

func toggleEntry(g *gocui.Gui, v *gocui.View) error {
	_, noViewErr := g.View("infoModal")
	if noViewErr != nil {
		showInfoModal(g, v)
	} else {
		hideInfoModal(g, v)
	}

	return nil
}

func setupKeybindings(repoInfos *[]RepoInfo, g *gocui.Gui) error {
	if err := g.SetKeybinding("", gocui.KeyCtrlC, gocui.ModNone, quit); err != nil {
		return err
	}
	if err := g.SetKeybinding("repoList", gocui.KeyCtrlN, gocui.ModNone, makeCursorReader(repoInfos, 1)); err != nil {
		return err
	}
	if err := g.SetKeybinding("repoList", gocui.KeyCtrlP, gocui.ModNone, makeCursorReader(repoInfos, -1)); err != nil {
		return err
	}
	if err := g.SetKeybinding("repoList", gocui.KeyArrowDown, gocui.ModNone, makeCursorReader(repoInfos, 1)); err != nil {
		return err
	}
	if err := g.SetKeybinding("repoList", gocui.KeyArrowUp, gocui.ModNone, makeCursorReader(repoInfos, -1)); err != nil {
		return err
	}
	if err := g.SetKeybinding("repoList", gocui.KeySpace, gocui.ModNone, toggleEntry); err != nil {
		return err
	}

	return nil
}

func quit(g *gocui.Gui, v *gocui.View) error {
	return gocui.ErrQuit
}

func main() {

	CWD, err := os.Getwd()
	if err != nil {
		log.Fatal(err)
	}

	maxDepth := flag.Int("maxdepth", 2, "max search depth in each provided directory")
	flag.Parse()

	// parse the remaining args, each one is a target dir
	dirs := flag.Args()
	if len(dirs) == 0 {
		dirs = append(dirs, CWD)
	}

	repoDirs := []string{}
	for _, dir := range dirs {
		FindRepos(dir, &repoDirs, *maxDepth)
	}

	repoInfos := []RepoInfo{}
	for _, repoDir := range repoDirs {
		repo, err := git.PlainOpen(repoDir)
		CheckIfError(err)

		ref, err := repo.Head()
		CheckIfError(err)

		commit, err := repo.CommitObject(ref.Hash())
		CheckIfError(err)

		/*
			// this doesn't tell if the repo is AHEAD of the remote
			// also if it is on a feature branch that is not the same as upstream
			// ... retrieving the commit object
			headRef, err := repo.Head()
			CheckIfError(err)

			headCommit, err := repo.CommitObject(headRef.Hash())
			CheckIfError(err)
				isAncestor, err := headCommit.IsAncestor(commit)
				CheckIfError(err)
				fmt.Printf("%s the HEAD an IsAncestor of origin/master? : %v\n",
					repoDir,
					isAncestor)
		*/

		worktree, _ := repo.Worktree()
		status, _ := worktree.Status()

		repoInfo := RepoInfo{
			path:       repoDir,
			branchName: ref.Name().String(),
			latestHash: ref.Hash().String(),
			time:       commit.Author.When,
			email:      commit.Author.Email,
			message:    commit.Message,
			isDirty:    !status.IsClean(),
		}
		repoInfos = append(repoInfos, repoInfo)

	}

	if len(repoInfos) == 0 {
		fmt.Println("no repos found")
	} else if false {
		fmt.Println(renderRepoInfoTableColored(&repoInfos))
	} else {
		fmt.Printf("found %d repos...\n", len(repoInfos))

		g, err := gocui.NewGui(gocui.OutputNormal)
		if err != nil {
			log.Panicln(err)
		}
		defer g.Close()

		g.Cursor = true
		g.SetManagerFunc(makeLayout(&repoInfos))

		if err := setupKeybindings(&repoInfos, g); err != nil {
			log.Panicln(err)
		}

		status := NewStatusbarWidget("status", 20, 15, 50)
		go update(g, status)

		go updateTable(g, &repoInfos)

		if err := g.MainLoop(); err != nil && err != gocui.ErrQuit {
			log.Panicln(err)
		}
	}
}
