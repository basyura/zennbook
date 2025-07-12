package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
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
		fmt.Print("fetch ...", c.Name, c.Url)
		if err := writeChapter(title, i+1, c); err != nil {
			return err
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

	body, err := ioutil.ReadAll(resp.Body)
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

	apiBody, err := ioutil.ReadAll(apiResp.Body)
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
	// Fetch chapter content from API
	apiUrl := fmt.Sprintf("https://zenn.dev/api/chapters/%d", c.ID)
	fmt.Printf("Fetching chapter content from: %s\n", apiUrl)

	resp, err := http.Get(apiUrl)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return err
	}

	var chapterData struct {
		Chapter struct {
			BodyHTML string `json:"body_html"`
		} `json:"chapter"`
	}

	if err := json.Unmarshal(body, &chapterData); err != nil {
		return err
	}

	// Create temporary HTML file
	tempFile := filepath.Join(os.TempDir(), fmt.Sprintf("temp_chapter_%d.html", c.ID))
	if err := os.WriteFile(tempFile, []byte(chapterData.Chapter.BodyHTML), 0644); err != nil {
		return err
	}
	defer os.Remove(tempFile)

	// Convert HTML to Markdown using html2md
	out, err := exec.Command("html2md", "-i", tempFile).Output()
	if err != nil {
		return err
	}

	lines := strings.Split("# "+strconv.Itoa(no)+". "+c.Name+"\n\n"+string(out), "\n")
	for i, line := range lines {
		if strings.HasPrefix(line, "```diff-") {
			lines[i] = "```diff"
		}
	}

	path := filepath.Join(title, fmt.Sprintf("chapter%02d.md", no))
	if err := os.WriteFile(path, []byte(strings.Join(lines, "\n")), os.ModePerm); err != nil {
		return err
	}

	return nil
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
	// fmt.Println("Result: " + out.String())
	if err != nil {
		return err
	}

	return err
}

func getFilePaths(baseDir string) ([]string, error) {
	files, err := ioutil.ReadDir(baseDir)

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
