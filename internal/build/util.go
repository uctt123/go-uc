















package build

import (
	"bytes"
	"flag"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"runtime"
	"strings"
	"text/template"
)

var DryRunFlag = flag.Bool("n", false, "dry run, don't execute commands")



func MustRun(cmd *exec.Cmd) {
	fmt.Println(">>>", strings.Join(cmd.Args, " "))
	if !*DryRunFlag {
		cmd.Stderr = os.Stderr
		cmd.Stdout = os.Stdout
		if err := cmd.Run(); err != nil {
			log.Fatal(err)
		}
	}
}

func MustRunCommand(cmd string, args ...string) {
	MustRun(exec.Command(cmd, args...))
}

var warnedAboutGit bool



func RunGit(args ...string) string {
	cmd := exec.Command("git", args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	if err := cmd.Run(); err != nil {
		if e, ok := err.(*exec.Error); ok && e.Err == exec.ErrNotFound {
			if !warnedAboutGit {
				log.Println("Warning: can't find 'git' in PATH")
				warnedAboutGit = true
			}
			return ""
		}
		log.Fatal(strings.Join(cmd.Args, " "), ": ", err, "\n", stderr.String())
	}
	return strings.TrimSpace(stdout.String())
}


func readGitFile(file string) string {
	content, err := ioutil.ReadFile(path.Join(".git", file))
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(content))
}


func Render(templateFile, outputFile string, outputPerm os.FileMode, x interface{}) {
	tpl := template.Must(template.ParseFiles(templateFile))
	render(tpl, outputFile, outputPerm, x)
}


func RenderString(templateContent, outputFile string, outputPerm os.FileMode, x interface{}) {
	tpl := template.Must(template.New("").Parse(templateContent))
	render(tpl, outputFile, outputPerm, x)
}

func render(tpl *template.Template, outputFile string, outputPerm os.FileMode, x interface{}) {
	if err := os.MkdirAll(filepath.Dir(outputFile), 0755); err != nil {
		log.Fatal(err)
	}
	out, err := os.OpenFile(outputFile, os.O_CREATE|os.O_WRONLY|os.O_EXCL, outputPerm)
	if err != nil {
		log.Fatal(err)
	}
	if err := tpl.Execute(out, x); err != nil {
		log.Fatal(err)
	}
	if err := out.Close(); err != nil {
		log.Fatal(err)
	}
}









func GoTool(tool string, args ...string) *exec.Cmd {
	args = append([]string{tool}, args...)
	return exec.Command(filepath.Join(runtime.GOROOT(), "bin", "go"), args...)
}




func UploadSFTP(identityFile, host, dir string, files []string) error {
	sftp := exec.Command("sftp")
	sftp.Stdout = nil
	sftp.Stderr = os.Stderr
	if identityFile != "" {
		sftp.Args = append(sftp.Args, "-i", identityFile)
	}
	sftp.Args = append(sftp.Args, host)
	fmt.Println(">>>", strings.Join(sftp.Args, " "))
	if *DryRunFlag {
		return nil
	}

	stdin, err := sftp.StdinPipe()
	if err != nil {
		return fmt.Errorf("can't create stdin pipe for sftp: %v", err)
	}
	if err := sftp.Start(); err != nil {
		return err
	}
	in := io.MultiWriter(stdin, os.Stdout)
	for _, f := range files {
		fmt.Fprintln(in, "put", f, path.Join(dir, filepath.Base(f)))
	}
	stdin.Close()
	return sftp.Wait()
}
