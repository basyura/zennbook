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

// balanceCodeBlockTags ensures proper balance between opening and closing code block tags
func balanceCodeBlockTags(lines []string) []string {
	var result []string
	openCount := 0
	closeCount := 0

	// First pass: count opens and closes
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "```") && len(trimmed) > 3 {
			openCount++
		} else if trimmed == "```" {
			closeCount++
		}
	}

	// Second pass: include appropriate number of closes
	actualCloses := 0
	maxCloses := openCount

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "```" {
			if actualCloses < maxCloses {
				result = append(result, line)
				actualCloses++
			}
			// Skip excess closes
		} else {
			result = append(result, line)
		}
	}

	return result
}

// formatCodeBlocks applies language-specific formatting to code blocks
func formatCodeBlocks(lines []string) []string {
	inCodeBlock := false
	codeBlockStart := -1
	codeBlockLang := ""

	for i := 0; i < len(lines); i++ {
		line := lines[i]
		trimmed := strings.TrimSpace(line)

		if strings.HasPrefix(trimmed, "```") {
			if !inCodeBlock {
				// Starting a code block
				inCodeBlock = true
				codeBlockStart = i
				if len(trimmed) > 3 {
					codeBlockLang = trimmed[3:]
				} else {
					codeBlockLang = ""
				}
			} else {
				// Ending a code block - format if language is specified
				inCodeBlock = false
				if codeBlockLang != "" && codeBlockStart >= 0 {
					formatted := formatCodeBlock(lines[codeBlockStart+1:i], codeBlockLang)
					if formatted != nil {
						// Replace the code block content with formatted version
						newLines := make([]string, 0, len(lines))
						newLines = append(newLines, lines[:codeBlockStart+1]...)
						newLines = append(newLines, formatted...)
						newLines = append(newLines, lines[i:]...)
						lines = newLines
						i = codeBlockStart + len(formatted)
					}
				}
				codeBlockStart = -1
				codeBlockLang = ""
			}
		}
	}

	return lines
}

// formatCodeBlock formats code using appropriate formatter based on language
func formatCodeBlock(codeLines []string, language string) []string {
	lang := strings.ToLower(language)

	// Join code lines
	code := strings.Join(codeLines, "\n")
	if strings.TrimSpace(code) == "" {
		return codeLines
	}

	// Try prettier for supported languages
	prettierLangs := map[string]string{
		"javascript": "babel",
		"js":         "babel",
		"typescript": "typescript",
		"ts":         "typescript",
		"jsx":        "babel",
		"tsx":        "typescript",
		"json":       "json",
		"css":        "css",
		"scss":       "scss",
		"less":       "less",
		"html":       "html",
		"markdown":   "markdown",
		"md":         "markdown",
		"yaml":       "yaml",
		"yml":        "yaml",
		"graphql":    "graphql",
	}

	if parser, supported := prettierLangs[lang]; supported {
		return formatWithPrettier(code, parser)
	}

	// Try other formatters for languages not supported by prettier
	switch lang {
	case "go":
		return formatWithGofmt(code)
	case "ruby", "rb":
		return formatWithRubocop(code)
	case "c#", "cs", "csharp":
		return formatWithDotnetFormat(code)
	case "python", "py":
		return formatWithBlack(code)
	case "rust", "rs":
		return formatWithRustfmt(code)
	case "java":
		return formatWithGoogleJavaFormat(code)
	default:
		// Language not supported, return original
		return nil
	}
}

// formatWithPrettier formats code using prettier
func formatWithPrettier(code, parser string) []string {
	tempFile := filepath.Join(os.TempDir(), fmt.Sprintf("temp_code_%d.tmp", len(code)))
	if err := os.WriteFile(tempFile, []byte(code), 0644); err != nil {
		fmt.Printf("Prettier formatting error: failed to create temp file: %v\n", err)
		return nil
	}
	defer os.Remove(tempFile)

	cmd := exec.Command("prettier", "--parser", parser, tempFile)
	output, err := cmd.Output()
	if err != nil {
		if exitError, ok := err.(*exec.ExitError); ok {
			fmt.Printf("Prettier formatting error (parser: %s): %v\nStderr: %s\n", parser, err, string(exitError.Stderr))
		} else {
			fmt.Printf("Prettier formatting error (parser: %s): %v\n", parser, err)
		}
		return nil
	}

	return strings.Split(strings.TrimSuffix(string(output), "\n"), "\n")
}

// formatWithGofmt formats Go code using gofmt
func formatWithGofmt(code string) []string {
	cmd := exec.Command("gofmt")
	cmd.Stdin = strings.NewReader(code)
	output, err := cmd.Output()
	if err != nil {
		if exitError, ok := err.(*exec.ExitError); ok {
			fmt.Printf("gofmt formatting error: %v\nStderr: %s\n", err, string(exitError.Stderr))
		} else {
			fmt.Printf("gofmt formatting error: %v\n", err)
		}
		return nil
	}

	return strings.Split(strings.TrimSuffix(string(output), "\n"), "\n")
}

// formatWithRubocop formats Ruby code using rubocop
func formatWithRubocop(code string) []string {
	tempFile := filepath.Join(os.TempDir(), fmt.Sprintf("temp_code_%d.rb", len(code)))
	if err := os.WriteFile(tempFile, []byte(code), 0644); err != nil {
		fmt.Printf("Rubocop formatting error: failed to create temp file: %v\n", err)
		return nil
	}
	defer os.Remove(tempFile)

	cmd := exec.Command("rubocop", "--auto-correct", "--stdin", tempFile)
	_, err := cmd.Output()
	if err != nil {
		if exitError, ok := err.(*exec.ExitError); ok {
			fmt.Printf("Rubocop formatting error: %v\nStderr: %s\n", err, string(exitError.Stderr))
		} else {
			fmt.Printf("Rubocop formatting error: %v\n", err)
		}
		return nil
	}

	// Read the corrected file
	corrected, err := os.ReadFile(tempFile)
	if err != nil {
		fmt.Printf("Rubocop formatting error: failed to read corrected file: %v\n", err)
		return nil
	}

	return strings.Split(strings.TrimSuffix(string(corrected), "\n"), "\n")
}

// formatWithDotnetFormat formats C# code using dotnet format
func formatWithDotnetFormat(code string) []string {
	tempFile := filepath.Join(os.TempDir(), fmt.Sprintf("temp_code_%d.cs", len(code)))
	if err := os.WriteFile(tempFile, []byte(code), 0644); err != nil {
		fmt.Printf("Dotnet format error: failed to create temp file: %v\n", err)
		return nil
	}
	defer os.Remove(tempFile)

	cmd := exec.Command("dotnet", "format", "--include", tempFile)
	if err := cmd.Run(); err != nil {
		if exitError, ok := err.(*exec.ExitError); ok {
			fmt.Printf("Dotnet format error: %v\nStderr: %s\n", err, string(exitError.Stderr))
		} else {
			fmt.Printf("Dotnet format error: %v\n", err)
		}
		return nil
	}

	formatted, err := os.ReadFile(tempFile)
	if err != nil {
		fmt.Printf("Dotnet format error: failed to read formatted file: %v\n", err)
		return nil
	}

	return strings.Split(strings.TrimSuffix(string(formatted), "\n"), "\n")
}

// formatWithBlack formats Python code using black
func formatWithBlack(code string) []string {
	cmd := exec.Command("black", "--code", code)
	output, err := cmd.Output()
	if err != nil {
		if exitError, ok := err.(*exec.ExitError); ok {
			fmt.Printf("Black formatting error: %v\nStderr: %s\n", err, string(exitError.Stderr))
		} else {
			fmt.Printf("Black formatting error: %v\n", err)
		}
		return nil
	}

	return strings.Split(strings.TrimSuffix(string(output), "\n"), "\n")
}

// formatWithRustfmt formats Rust code using rustfmt
func formatWithRustfmt(code string) []string {
	cmd := exec.Command("rustfmt", "--emit", "stdout")
	cmd.Stdin = strings.NewReader(code)
	output, err := cmd.Output()
	if err != nil {
		if exitError, ok := err.(*exec.ExitError); ok {
			fmt.Printf("Rustfmt formatting error: %v\nStderr: %s\n", err, string(exitError.Stderr))
		} else {
			fmt.Printf("Rustfmt formatting error: %v\n", err)
		}
		return nil
	}

	return strings.Split(strings.TrimSuffix(string(output), "\n"), "\n")
}

// formatWithGoogleJavaFormat formats Java code using google-java-format
func formatWithGoogleJavaFormat(code string) []string {
	cmd := exec.Command("google-java-format", "-")
	cmd.Stdin = strings.NewReader(code)
	output, err := cmd.Output()
	if err != nil {
		if exitError, ok := err.(*exec.ExitError); ok {
			fmt.Printf("Google Java Format error: %v\nStderr: %s\n", err, string(exitError.Stderr))
		} else {
			fmt.Printf("Google Java Format error: %v\n", err)
		}
		return nil
	}

	return strings.Split(strings.TrimSuffix(string(output), "\n"), "\n")
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
