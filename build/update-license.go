

















/*
This command generates GPL license headers on top of all source files.
You can run it once per month, before cutting a release or just
whenever you feel like it.

	go run update-license.go

All authors (people who have contributed code) are listed in the
AUTHORS file. The author names are mapped and deduplicated using the
.mailmap file. You can use .mailmap to set the canonical name and
address for each author. See git-shortlog(1) for an explanation of the
.mailmap format.

Please review the resulting diff to check whether the correct
copyright assignments are performed.
*/

package main

import (
	"bufio"
	"bytes"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
	"text/template"
	"time"
)

var (
	
	extensions = []string{".go", ".js", ".qml"}

	
	skipPrefixes = []string{
		
		"vendor/", "tests/testdata/", "build/",

		
		"cmd/internal/browser",
		"common/bitutil/bitutil",
		"common/prque/",
		"consensus/ethash/xor.go",
		"crypto/bn256/",
		"crypto/ecies/",
		"graphql/graphiql.go",
		"internal/jsre/deps",
		"log/",
		"metrics/",
		"signer/rules/deps",

		
		"crypto/secp256k1", 
	}

	
	gplPrefixes = []string{"cmd/"}

	
	
	licenseCommentRE = regexp.MustCompile(`^

	
	authorsFileHeader = "# This is the official list of go-ethereum authors for copyright purposes.\n\n"
)



var licenseT = template.Must(template.New("").Parse(`
















`[1:]))

type info struct {
	file string
	Year int64
}

func (i info) License() string {
	if i.gpl() {
		return "General Public License"
	}
	return "Lesser General Public License"
}

func (i info) ShortLicense() string {
	if i.gpl() {
		return "GPL"
	}
	return "LGPL"
}

func (i info) Whole(startOfSentence bool) string {
	if i.gpl() {
		return "go-ethereum"
	}
	if startOfSentence {
		return "The go-ethereum library"
	}
	return "the go-ethereum library"
}

func (i info) gpl() bool {
	for _, p := range gplPrefixes {
		if strings.HasPrefix(i.file, p) {
			return true
		}
	}
	return false
}


type authors []string

func (as authors) Len() int           { return len(as) }
func (as authors) Less(i, j int) bool { return strings.ToLower(as[i]) < strings.ToLower(as[j]) }
func (as authors) Swap(i, j int)      { as[i], as[j] = as[j], as[i] }

func main() {
	var (
		files = getFiles()
		filec = make(chan string)
		infoc = make(chan *info, 20)
		wg    sync.WaitGroup
	)

	writeAuthors(files)

	go func() {
		for _, f := range files {
			filec <- f
		}
		close(filec)
	}()
	for i := runtime.NumCPU(); i >= 0; i-- {
		
		
		wg.Add(1)
		go getInfo(filec, infoc, &wg)
	}
	go func() {
		wg.Wait()
		close(infoc)
	}()
	writeLicenses(infoc)
}

func skipFile(path string) bool {
	if strings.Contains(path, "/testdata/") {
		return true
	}
	for _, p := range skipPrefixes {
		if strings.HasPrefix(path, p) {
			return true
		}
	}
	return false
}

func getFiles() []string {
	cmd := exec.Command("git", "ls-tree", "-r", "--name-only", "HEAD")
	var files []string
	err := doLines(cmd, func(line string) {
		if skipFile(line) {
			return
		}
		ext := filepath.Ext(line)
		for _, wantExt := range extensions {
			if ext == wantExt {
				goto keep
			}
		}
		return
	keep:
		files = append(files, line)
	})
	if err != nil {
		log.Fatal("error getting files:", err)
	}
	return files
}

var authorRegexp = regexp.MustCompile(`\s*[0-9]+\s*(.*)`)

func gitAuthors(files []string) []string {
	cmds := []string{"shortlog", "-s", "-n", "-e", "HEAD", "--"}
	cmds = append(cmds, files...)
	cmd := exec.Command("git", cmds...)
	var authors []string
	err := doLines(cmd, func(line string) {
		m := authorRegexp.FindStringSubmatch(line)
		if len(m) > 1 {
			authors = append(authors, m[1])
		}
	})
	if err != nil {
		log.Fatalln("error getting authors:", err)
	}
	return authors
}

func readAuthors() []string {
	content, err := ioutil.ReadFile("AUTHORS")
	if err != nil && !os.IsNotExist(err) {
		log.Fatalln("error reading AUTHORS:", err)
	}
	var authors []string
	for _, a := range bytes.Split(content, []byte("\n")) {
		if len(a) > 0 && a[0] != '#' {
			authors = append(authors, string(a))
		}
	}
	
	
	authors = mailmapLookup(authors)
	return authors
}

func mailmapLookup(authors []string) []string {
	if len(authors) == 0 {
		return nil
	}
	cmds := []string{"check-mailmap", "--"}
	cmds = append(cmds, authors...)
	cmd := exec.Command("git", cmds...)
	var translated []string
	err := doLines(cmd, func(line string) {
		translated = append(translated, line)
	})
	if err != nil {
		log.Fatalln("error translating authors:", err)
	}
	return translated
}

func writeAuthors(files []string) {
	var (
		dedup = make(map[string]bool)
		list  []string
	)
	
	
	for _, a := range gitAuthors(files) {
		if la := strings.ToLower(a); !dedup[la] {
			list = append(list, a)
			dedup[la] = true
		}
	}
	
	
	
	for _, a := range readAuthors() {
		if la := strings.ToLower(a); !dedup[la] {
			list = append(list, a)
			dedup[la] = true
		}
	}
	
	sort.Sort(authors(list))
	content := new(bytes.Buffer)
	content.WriteString(authorsFileHeader)
	for _, a := range list {
		content.WriteString(a)
		content.WriteString("\n")
	}
	fmt.Println("writing AUTHORS")
	if err := ioutil.WriteFile("AUTHORS", content.Bytes(), 0644); err != nil {
		log.Fatalln(err)
	}
}

func getInfo(files <-chan string, out chan<- *info, wg *sync.WaitGroup) {
	for file := range files {
		stat, err := os.Lstat(file)
		if err != nil {
			fmt.Printf("ERROR %s: %v\n", file, err)
			continue
		}
		if !stat.Mode().IsRegular() {
			continue
		}
		if isGenerated(file) {
			continue
		}
		info, err := fileInfo(file)
		if err != nil {
			fmt.Printf("ERROR %s: %v\n", file, err)
			continue
		}
		out <- info
	}
	wg.Done()
}

func isGenerated(file string) bool {
	fd, err := os.Open(file)
	if err != nil {
		return false
	}
	defer fd.Close()
	buf := make([]byte, 2048)
	n, _ := fd.Read(buf)
	buf = buf[:n]
	for _, l := range bytes.Split(buf, []byte("\n")) {
		if bytes.HasPrefix(l, []byte("
			return true
		}
	}
	return false
}


func fileInfo(file string) (*info, error) {
	info := &info{file: file, Year: int64(time.Now().Year())}
	cmd := exec.Command("git", "log", "--follow", "--find-renames=80", "--find-copies=80", "--pretty=format:%ai", "--", file)
	err := doLines(cmd, func(line string) {
		y, err := strconv.ParseInt(line[:4], 10, 64)
		if err != nil {
			fmt.Printf("cannot parse year: %q", line[:4])
		}
		if y < info.Year {
			info.Year = y
		}
	})
	return info, err
}

func writeLicenses(infos <-chan *info) {
	for i := range infos {
		writeLicense(i)
	}
}

func writeLicense(info *info) {
	fi, err := os.Stat(info.file)
	if os.IsNotExist(err) {
		fmt.Println("skipping (does not exist)", info.file)
		return
	}
	if err != nil {
		log.Fatalf("error stat'ing %s: %v\n", info.file, err)
	}
	content, err := ioutil.ReadFile(info.file)
	if err != nil {
		log.Fatalf("error reading %s: %v\n", info.file, err)
	}
	
	buf := new(bytes.Buffer)
	licenseT.Execute(buf, info)
	if m := licenseCommentRE.FindIndex(content); m != nil && m[0] == 0 {
		buf.Write(content[:m[0]])
		buf.Write(content[m[1]:])
	} else {
		buf.Write(content)
	}
	
	if bytes.Equal(content, buf.Bytes()) {
		fmt.Println("skipping (no changes)", info.file)
		return
	}
	fmt.Println("writing", info.ShortLicense(), info.file)
	if err := ioutil.WriteFile(info.file, buf.Bytes(), fi.Mode()); err != nil {
		log.Fatalf("error writing %s: %v", info.file, err)
	}
}

func doLines(cmd *exec.Cmd, f func(string)) error {
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}
	if err := cmd.Start(); err != nil {
		return err
	}
	s := bufio.NewScanner(stdout)
	for s.Scan() {
		f(s.Text())
	}
	if s.Err() != nil {
		return s.Err()
	}
	if err := cmd.Wait(); err != nil {
		return fmt.Errorf("%v (for %s)", err, strings.Join(cmd.Args, " "))
	}
	return nil
}
