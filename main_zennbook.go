package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"

	. "github.com/basyura/zennbook/models"
)

func main() {

	if len(os.Args) < 3 {
		fmt.Println("zennbook hoge/books/012345 title")
		return
	}

	id := os.Args[1]
	title := os.Args[2]
	css := ""
	if len(os.Args) > 4 {
		css = os.Args[3]
	}

	if strings.HasPrefix(id, "http") {
		id = strings.Replace(id, "https://zenn.dev/", "", 1)
	}

	if err := doMain(id, title, css); err != nil {
		fmt.Println(err)
	}
}

func doMain(id string, title string, css string) error {

	os.Mkdir(title, os.ModePerm)

	chapters, err := parseChapters(id)
	if err != nil {
		return err
	}

	for i, c := range chapters {
		fmt.Printf("fetch ... %s (ID: %d) %s\n", c.Name, c.ID, c.Url)
		if err := writeChapter(title, i+1, c); err != nil {
			fmt.Printf("Error processing chapter %d: %v\n", i+1, err)
			// Continue with next chapter instead of stopping
			continue
		}
		fmt.Println("... end")
	}

	// manual
	// $ pandoc -f markdown *.md -o hoge.epub --metadata title="ほげ"
	if err := pandoc(title, css); err != nil {
		return err
	}

	return nil
}

func parseChapters(id string) ([]Chapter, error) {
	url := "https://zenn.dev/" + id
	fmt.Println("fetch :", url)

	// Extract buildId from main page
	resp, err := http.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	// Extract buildId from script tag
	htmlContent := string(body)
	buildIdStart := strings.Index(htmlContent, `"buildId":"`)
	if buildIdStart == -1 {
		return nil, fmt.Errorf("buildId not found")
	}
	buildIdStart += len(`"buildId":"`)
	buildIdEnd := strings.Index(htmlContent[buildIdStart:], `"`)
	if buildIdEnd == -1 {
		return nil, fmt.Errorf("buildId end not found")
	}
	buildId := htmlContent[buildIdStart : buildIdStart+buildIdEnd]

	// Fetch chapter data from Next.js API
	apiUrl := fmt.Sprintf("https://zenn.dev/_next/data/%s/%s.json", buildId, id)
	fmt.Println("API fetch:", apiUrl)

	apiResp, err := http.Get(apiUrl)
	if err != nil {
		return nil, err
	}
	defer apiResp.Body.Close()

	apiBody, err := io.ReadAll(apiResp.Body)
	if err != nil {
		return nil, err
	}

	var apiData struct {
		PageProps struct {
			Chapters []Chapter `json:"chapters"`
		} `json:"pageProps"`
	}

	if err := json.Unmarshal(apiBody, &apiData); err != nil {
		return nil, err
	}

	// Set URLs for chapters
	for i := range apiData.PageProps.Chapters {
		chapter := &apiData.PageProps.Chapters[i]
		chapter.Url = "https://zenn.dev" + chapter.Url
		fmt.Println(chapter.Name, chapter.Url)
	}

	return apiData.PageProps.Chapters, nil
}

func writeChapter(title string, no int, c Chapter) error {
	// Fetch and convert chapter content
	content, err := fetchAndConvertChapter(c, no)
	if err != nil {
		return err
	}

	// Process and format the content
	processedContent := processContent(content)

	// Write to file
	path := filepath.Join(title, fmt.Sprintf("chapter%02d.md", no))
	if err := os.WriteFile(path, []byte(strings.Join(processedContent, "\n")), os.ModePerm); err != nil {
		return err
	}

	return nil
}

// fetchAndConvertChapter fetches chapter content from API and converts HTML to Markdown
func fetchAndConvertChapter(c Chapter, no int) (string, error) {
	// Fetch chapter content from API
	apiUrl := fmt.Sprintf("https://zenn.dev/api/chapters/%d", c.ID)
	fmt.Printf("Fetching chapter content from: %s\n", apiUrl)

	resp, err := http.Get(apiUrl)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	var chapterData struct {
		Chapter struct {
			BodyHTML string `json:"body_html"`
		} `json:"chapter"`
	}

	if err := json.Unmarshal(body, &chapterData); err != nil {
		return "", err
	}

	// Create temporary HTML file
	tempFile := filepath.Join(os.TempDir(), fmt.Sprintf("temp_chapter_%d.html", c.ID))
	if err := os.WriteFile(tempFile, []byte(chapterData.Chapter.BodyHTML), 0644); err != nil {
		return "", err
	}
	defer os.Remove(tempFile)

	// Convert HTML to Markdown using html2md
	out, err := exec.Command("html2md", "-i", tempFile).Output()
	if err != nil {
		return "", err
	}

	content := "# " + strconv.Itoa(no) + ". " + c.Name + "\n\n" + string(out)
	return content, nil
}

// processContent applies all content processing steps
func processContent(content string) []string {
	// Clean HTML tokens
	content = cleanHTMLTokens(content)

	// Split into lines for processing
	lines := strings.Split(content, "\n")

	// Fix code blocks
	lines = fixCodeBlocks(lines)

	// Balance code block tags
	//lines = balanceCodeBlockTags(lines)

	// Format code blocks
	//lines = formatCodeBlocks(lines)

	return lines
}

// cleanHTMLTokens removes unwanted HTML tokens from content
func cleanHTMLTokens(content string) string {
	content = strings.ReplaceAll(content, "<span class=\"token builtin class-name\">", "")
	content = strings.ReplaceAll(content, "<span class=\"token function\">", "")
	content = strings.ReplaceAll(content, "</span>", "")
	return content
}

// fixCodeBlocks processes code block formatting issues
func fixCodeBlocks(lines []string) []string {
	for i := 0; i < len(lines); i++ {
		line := lines[i]

		if strings.HasPrefix(line, "```diff-") {
			lines[i] = "```diff"
		}

		// Fix code blocks ending with " code-line"
		if strings.Contains(line, "```") && strings.HasSuffix(line, " code-line") {
			lines = processCodeLineSuffix(lines, i)
		}
	}
	return lines
}

// processCodeLineSuffix handles code blocks with " code-line" suffix
func processCodeLineSuffix(lines []string, i int) []string {
	// Remove " code-line" from the end
	lines[i] = strings.TrimSuffix(lines[i], " code-line")

	// Check if there are commands on subsequent lines
	commandLines := []string{}
	j := i + 1

	// Collect all command lines until we find an empty line or end of lines
	for j < len(lines) && strings.TrimSpace(lines[j]) != "" && !strings.HasPrefix(strings.TrimSpace(lines[j]), "```") {
		commandLines = append(commandLines, strings.TrimSuffix(lines[j], " "))
		j++
	}

	if len(commandLines) > 0 {
		// Process and split commands if needed
		finalCommands := splitShellCommands(commandLines)

		// Replace the original command lines with split commands
		// First, remove original command lines
		lines = append(lines[:i+1], lines[j:]...)

		// Insert the split commands
		for k, cmd := range finalCommands {
			lines = append(lines[:i+1+k], append([]string{cmd}, lines[i+1+k:]...)...)
		}
	}

	return lines
}

// splitShellCommands splits concatenated shell commands into separate lines
func splitShellCommands(commandLines []string) []string {
	allCommands := strings.Join(commandLines, " ")
	var finalCommands []string

	// Handle specific patterns for shell commands
	if strings.Contains(allCommands, "mkdir") || strings.Contains(allCommands, "cd") || strings.Contains(allCommands, "go ") {
		// Split by common command keywords
		parts := strings.Fields(allCommands)
		currentCmd := ""
		for _, part := range parts {
			if part == "mkdir" || part == "cd" || part == "go" || part == "npm" || part == "git" || part == "echo" || part == "export" {
				if currentCmd != "" {
					finalCommands = append(finalCommands, currentCmd)
				}
				currentCmd = part
			} else {
				if currentCmd != "" {
					currentCmd += " " + part
				} else {
					currentCmd = part
				}
			}
		}
		if currentCmd != "" {
			finalCommands = append(finalCommands, currentCmd)
		}
	} else {
		// Keep original command lines
		finalCommands = commandLines
	}

	return finalCommands
}

func pandoc(title string, css string) error {

	// $ pandoc -f markdown *.md -o hoge.epub --metadata title="ほげ"
	args := []string{
		"-f", "markdown",
		"-o", filepath.Join(title, title+".epub"),
		"--metadata", "title=" + title,
	}

	// "--css", "~/.kindle/KPR/style.css",
	// home, err := os.UserHomeDir()
	// if err != nil {
	// 	return err
	// }
	// args = append(args, []string{"--css", filepath.Join(home, ".kindle/KPR/style.css")}...)

	if css != "" {
		args = append(args, []string{"--css", css}...)
	}

	paths, err := getFilePaths(title)
	if err != nil {
		return err
	}
	for _, f := range paths {
		args = append(args, f)
	}

	cmd := exec.Command("pandoc", args...)
	fmt.Println(cmd)

	var out bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &stderr
	err = cmd.Run()
	if err != nil {
		fmt.Println(fmt.Sprint(err) + ": " + stderr.String())
		return err
	}

	return err
}

func getFilePaths(baseDir string) ([]string, error) {
	files, err := os.ReadDir(baseDir)

	if err != nil {
		fmt.Println("read error :", baseDir)
		os.Exit(1)
	}

	var paths []string
	for _, file := range files {

		if filepath.Ext(file.Name()) == ".md" {
			path, err := filepath.Abs(filepath.Join(baseDir, file.Name()))
			if err != nil {
				return nil, err
			}
			paths = append(paths, path)
		}
	}

	return paths, nil
}
