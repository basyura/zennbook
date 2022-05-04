package main

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	. "zennbook/models"
)

func main() {

	if len(os.Args) < 3 {
		fmt.Println("zennbook hoge/books/012345 title")
		return
	}

	id := os.Args[1]
	title := os.Args[2]

	if strings.HasPrefix(id, "http") {
		id = strings.Replace(id, "https://zenn.dev/", "", 1)
	}

	if err := doMain(id, title); err != nil {
		fmt.Println(err)
	}
}

func doMain(id string, title string) error {
	chapters, err := parseChapters(id)
	if err != nil {
		return err
	}

	for i, c := range chapters {
		fmt.Print("fetch ...", c.Name, c.Url)
		if err := writeChapter(i+1, c); err != nil {
			return err
		}
		fmt.Println("... end")
	}

	// manual
	// $ pandoc -f markdown *.md -o hoge.epub --metadata title="ほげ"
	if err := pandoc(title); err != nil {
		return err
	}

	return nil
}

func parseChapters(id string) ([]Chapter, error) {

	url := "https://zenn.dev/" + id
	fmt.Println("fetch :", url)

	out, err := exec.Command("html2md", "-i", url, "-s", "#chapters").Output()
	if err != nil {
		return nil, err
	}

	content := string(out)
	content = strings.ReplaceAll(content, " [Chapter ", "\n[Chapter ")
	content = strings.ReplaceAll(content, "**", "")
	content = strings.ReplaceAll(content, "](", "\t")
	content = strings.ReplaceAll(content, ")", "")
	content = strings.ReplaceAll(content, " ", "")

	chapters := []Chapter{}
	for _, s := range strings.Split(content, "\n") {
		if strings.Contains(s, "http") {
			pair := strings.Split(s, "\t")
			c := Chapter{Name: pair[0], Url: pair[1]}
			chapters = append(chapters, c)
			fmt.Println(c.Name, c.Url)
		}
	}

	return chapters, nil
}

func writeChapter(no int, c Chapter) error {

	out, err := exec.Command("html2md", "-i", c.Url, "-s", "#viewer-toc").Output()
	if err != nil {
		return err
	}

	s := "# " + c.Name + "\n\n" + string(out)

	path := filepath.Join(fmt.Sprintf("chapter%02d.md", no))
	if err := os.WriteFile(path, []byte(s), os.ModePerm); err != nil {
		return err
	}

	return nil
}

func pandoc(title string) error {

	// $ pandoc -f markdown *.md -o hoge.epub --metadata title="ほげ"
	args := []string{
		"-f", "markdown",
		"-o", title + ".epub",
		"--metadata", "title=" + title,
	}

	paths, err := getFilePaths(".")
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
