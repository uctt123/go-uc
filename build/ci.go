

















/*
The ci command is called from Continuous Integration scripts.

Usage: go run build/ci.go <command> <command flags/arguments>

Available commands are:

   install    [ -arch architecture ] [ -cc compiler ] [ packages... ]                          -- builds packages and executables
   test       [ -coverage ] [ packages... ]                                                    -- runs the tests
   lint                                                                                        -- runs certain pre-selected linters
   archive    [ -arch architecture ] [ -type zip|tar ] [ -signer key-envvar ] [ -upload dest ] -- archives build artifacts
   importkeys                                                                                  -- imports signing keys from env
   debsrc     [ -signer key-id ] [ -upload dest ]                                              -- creates a debian source package
   nsis                                                                                        -- creates a Windows NSIS installer
   aar        [ -local ] [ -sign key-id ] [-deploy repo] [ -upload dest ]                      -- creates an Android archive
   xcode      [ -local ] [ -sign key-id ] [-deploy repo] [ -upload dest ]                      -- creates an iOS XCode framework
   xgo        [ -alltools ] [ options ]                                                        -- cross builds according to options
   purge      [ -store blobstore ] [ -days threshold ]                                         -- purges old archives from the blobstore

For all commands, -n prevents execution of external programs (dry run mode).

*/
package main

import (
	"bufio"
	"bytes"
	"encoding/base64"
	"flag"
	"fmt"
	"go/parser"
	"go/token"
	"io/ioutil"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"time"

	"github.com/cespare/cp"
	"github.com/ethereum/go-ethereum/internal/build"
	"github.com/ethereum/go-ethereum/params"
)

var (
	
	gethArchiveFiles = []string{
		"COPYING",
		executablePath("geth"),
	}

	
	allToolsArchiveFiles = []string{
		"COPYING",
		executablePath("abigen"),
		executablePath("bootnode"),
		executablePath("evm"),
		executablePath("geth"),
		executablePath("puppeth"),
		executablePath("rlpdump"),
		executablePath("clef"),
	}

	
	debExecutables = []debExecutable{
		{
			BinaryName:  "abigen",
			Description: "Source code generator to convert Ethereum contract definitions into easy to use, compile-time type-safe Go packages.",
		},
		{
			BinaryName:  "bootnode",
			Description: "Ethereum bootnode.",
		},
		{
			BinaryName:  "evm",
			Description: "Developer utility version of the EVM (Ethereum Virtual Machine) that is capable of running bytecode snippets within a configurable environment and execution mode.",
		},
		{
			BinaryName:  "geth",
			Description: "Ethereum CLI client.",
		},
		{
			BinaryName:  "puppeth",
			Description: "Ethereum private network manager.",
		},
		{
			BinaryName:  "rlpdump",
			Description: "Developer utility tool that prints RLP structures.",
		},
		{
			BinaryName:  "clef",
			Description: "Ethereum account management tool.",
		},
	}

	

	debEthereum = debPackage{
		Name:        "ethereum",
		Version:     params.Version,
		Executables: debExecutables,
	}

	
	debPackages = []debPackage{
		debEthereum,
	}

	
	
	
	
	
	
	
	
	debDistroGoBoots = map[string]string{
		"trusty": "golang-1.11",
		"xenial": "golang-go",
		"bionic": "golang-go",
		"eoan":   "golang-go",
		"focal":  "golang-go",
		"groovy": "golang-go",
	}

	debGoBootPaths = map[string]string{
		"golang-1.11": "/usr/lib/go-1.11",
		"golang-go":   "/usr/lib/go",
	}
)

var GOBIN, _ = filepath.Abs(filepath.Join("build", "bin"))

func executablePath(name string) string {
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	return filepath.Join(GOBIN, name)
}

func main() {
	log.SetFlags(log.Lshortfile)

	if _, err := os.Stat(filepath.Join("build", "ci.go")); os.IsNotExist(err) {
		log.Fatal("this script must be run from the root of the repository")
	}
	if len(os.Args) < 2 {
		log.Fatal("need subcommand as first argument")
	}
	switch os.Args[1] {
	case "install":
		doInstall(os.Args[2:])
	case "test":
		doTest(os.Args[2:])
	case "lint":
		doLint(os.Args[2:])
	case "archive":
		doArchive(os.Args[2:])
	case "debsrc":
		doDebianSource(os.Args[2:])
	case "nsis":
		doWindowsInstaller(os.Args[2:])
	case "aar":
		doAndroidArchive(os.Args[2:])
	case "xcode":
		doXCodeFramework(os.Args[2:])
	case "xgo":
		doXgo(os.Args[2:])
	case "purge":
		doPurge(os.Args[2:])
	default:
		log.Fatal("unknown command ", os.Args[1])
	}
}



func doInstall(cmdline []string) {
	var (
		arch = flag.String("arch", "", "Architecture to cross build for")
		cc   = flag.String("cc", "", "C compiler to cross build with")
	)
	flag.CommandLine.Parse(cmdline)
	env := build.Env()

	
	
	if !strings.Contains(runtime.Version(), "devel") {
		
		var minor int
		fmt.Sscanf(strings.TrimPrefix(runtime.Version(), "go1."), "%d", &minor)

		if minor < 13 {
			log.Println("You have Go version", runtime.Version())
			log.Println("go-ethereum requires at least Go version 1.13 and cannot")
			log.Println("be compiled with an earlier version. Please upgrade your Go installation.")
			os.Exit(1)
		}
	}
	
	packages := []string{"./..."}
	if flag.NArg() > 0 {
		packages = flag.Args()
	}

	if *arch == "" || *arch == runtime.GOARCH {
		goinstall := goTool("install", buildFlags(env)...)
		if runtime.GOARCH == "arm64" {
			goinstall.Args = append(goinstall.Args, "-p", "1")
		}
		goinstall.Args = append(goinstall.Args, "-trimpath")
		goinstall.Args = append(goinstall.Args, "-v")
		goinstall.Args = append(goinstall.Args, packages...)
		build.MustRun(goinstall)
		return
	}

	
	goinstall := goToolArch(*arch, *cc, "install", buildFlags(env)...)
	goinstall.Args = append(goinstall.Args, "-trimpath")
	goinstall.Args = append(goinstall.Args, "-v")
	goinstall.Args = append(goinstall.Args, []string{"-buildmode", "archive"}...)
	goinstall.Args = append(goinstall.Args, packages...)
	build.MustRun(goinstall)

	if cmds, err := ioutil.ReadDir("cmd"); err == nil {
		for _, cmd := range cmds {
			pkgs, err := parser.ParseDir(token.NewFileSet(), filepath.Join(".", "cmd", cmd.Name()), nil, parser.PackageClauseOnly)
			if err != nil {
				log.Fatal(err)
			}
			for name := range pkgs {
				if name == "main" {
					gobuild := goToolArch(*arch, *cc, "build", buildFlags(env)...)
					gobuild.Args = append(gobuild.Args, "-v")
					gobuild.Args = append(gobuild.Args, []string{"-o", executablePath(cmd.Name())}...)
					gobuild.Args = append(gobuild.Args, "."+string(filepath.Separator)+filepath.Join("cmd", cmd.Name()))
					build.MustRun(gobuild)
					break
				}
			}
		}
	}
}

func buildFlags(env build.Environment) (flags []string) {
	var ld []string
	if env.Commit != "" {
		ld = append(ld, "-X", "main.gitCommit="+env.Commit)
		ld = append(ld, "-X", "main.gitDate="+env.Date)
	}
	if runtime.GOOS == "darwin" {
		ld = append(ld, "-s")
	}

	if len(ld) > 0 {
		flags = append(flags, "-ldflags", strings.Join(ld, " "))
	}
	return flags
}

func goTool(subcmd string, args ...string) *exec.Cmd {
	return goToolArch(runtime.GOARCH, os.Getenv("CC"), subcmd, args...)
}

func goToolArch(arch string, cc string, subcmd string, args ...string) *exec.Cmd {
	cmd := build.GoTool(subcmd, args...)
	if arch == "" || arch == runtime.GOARCH {
		cmd.Env = append(cmd.Env, "GOBIN="+GOBIN)
	} else {
		cmd.Env = append(cmd.Env, "CGO_ENABLED=1")
		cmd.Env = append(cmd.Env, "GOARCH="+arch)
	}
	if cc != "" {
		cmd.Env = append(cmd.Env, "CC="+cc)
	}
	for _, e := range os.Environ() {
		if strings.HasPrefix(e, "GOBIN=") {
			continue
		}
		cmd.Env = append(cmd.Env, e)
	}
	return cmd
}





func doTest(cmdline []string) {
	coverage := flag.Bool("coverage", false, "Whether to record code coverage")
	verbose := flag.Bool("v", false, "Whether to log verbosely")
	flag.CommandLine.Parse(cmdline)
	env := build.Env()

	packages := []string{"./..."}
	if len(flag.CommandLine.Args()) > 0 {
		packages = flag.CommandLine.Args()
	}

	
	
	
	gotest := goTool("test", buildFlags(env)...)
	gotest.Args = append(gotest.Args, "-p", "1")
	if *coverage {
		gotest.Args = append(gotest.Args, "-covermode=atomic", "-cover")
	}
	if *verbose {
		gotest.Args = append(gotest.Args, "-v")
	}

	gotest.Args = append(gotest.Args, packages...)
	build.MustRun(gotest)
}


func doLint(cmdline []string) {
	var (
		cachedir = flag.String("cachedir", "./build/cache", "directory for caching golangci-lint binary.")
	)
	flag.CommandLine.Parse(cmdline)
	packages := []string{"./..."}
	if len(flag.CommandLine.Args()) > 0 {
		packages = flag.CommandLine.Args()
	}

	linter := downloadLinter(*cachedir)
	lflags := []string{"run", "--config", ".golangci.yml"}
	build.MustRunCommand(linter, append(lflags, packages...)...)
	fmt.Println("You have achieved perfection.")
}


func downloadLinter(cachedir string) string {
	const version = "1.27.0"

	csdb := build.MustLoadChecksums("build/checksums.txt")
	base := fmt.Sprintf("golangci-lint-%s-%s-%s", version, runtime.GOOS, runtime.GOARCH)
	url := fmt.Sprintf("https:
	archivePath := filepath.Join(cachedir, base+".tar.gz")
	if err := csdb.DownloadFile(url, archivePath); err != nil {
		log.Fatal(err)
	}
	if err := build.ExtractTarballArchive(archivePath, cachedir); err != nil {
		log.Fatal(err)
	}
	return filepath.Join(cachedir, base, "golangci-lint")
}


func doArchive(cmdline []string) {
	var (
		arch   = flag.String("arch", runtime.GOARCH, "Architecture cross packaging")
		atype  = flag.String("type", "zip", "Type of archive to write (zip|tar)")
		signer = flag.String("signer", "", `Environment variable holding the signing key (e.g. LINUX_SIGNING_KEY)`)
		upload = flag.String("upload", "", `Destination to upload the archives (usually "gethstore/builds")`)
		ext    string
	)
	flag.CommandLine.Parse(cmdline)
	switch *atype {
	case "zip":
		ext = ".zip"
	case "tar":
		ext = ".tar.gz"
	default:
		log.Fatal("unknown archive type: ", atype)
	}

	var (
		env = build.Env()

		basegeth = archiveBasename(*arch, params.ArchiveVersion(env.Commit))
		geth     = "geth-" + basegeth + ext
		alltools = "geth-alltools-" + basegeth + ext
	)
	maybeSkipArchive(env)
	if err := build.WriteArchive(geth, gethArchiveFiles); err != nil {
		log.Fatal(err)
	}
	if err := build.WriteArchive(alltools, allToolsArchiveFiles); err != nil {
		log.Fatal(err)
	}
	for _, archive := range []string{geth, alltools} {
		if err := archiveUpload(archive, *upload, *signer); err != nil {
			log.Fatal(err)
		}
	}
}

func archiveBasename(arch string, archiveVersion string) string {
	platform := runtime.GOOS + "-" + arch
	if arch == "arm" {
		platform += os.Getenv("GOARM")
	}
	if arch == "android" {
		platform = "android-all"
	}
	if arch == "ios" {
		platform = "ios-all"
	}
	return platform + "-" + archiveVersion
}

func archiveUpload(archive string, blobstore string, signer string) error {
	
	if signer != "" {
		key := getenvBase64(signer)
		if err := build.PGPSignFile(archive, archive+".asc", string(key)); err != nil {
			return err
		}
	}
	
	if blobstore != "" {
		auth := build.AzureBlobstoreConfig{
			Account:   strings.Split(blobstore, "/")[0],
			Token:     os.Getenv("AZURE_BLOBSTORE_TOKEN"),
			Container: strings.SplitN(blobstore, "/", 2)[1],
		}
		if err := build.AzureBlobstoreUpload(archive, filepath.Base(archive), auth); err != nil {
			return err
		}
		if signer != "" {
			if err := build.AzureBlobstoreUpload(archive+".asc", filepath.Base(archive+".asc"), auth); err != nil {
				return err
			}
		}
	}
	return nil
}


func maybeSkipArchive(env build.Environment) {
	if env.IsPullRequest {
		log.Printf("skipping because this is a PR build")
		os.Exit(0)
	}
	if env.IsCronJob {
		log.Printf("skipping because this is a cron job")
		os.Exit(0)
	}
	if env.Branch != "master" && !strings.HasPrefix(env.Tag, "v1.") {
		log.Printf("skipping because branch %q, tag %q is not on the whitelist", env.Branch, env.Tag)
		os.Exit(0)
	}
}


func doDebianSource(cmdline []string) {
	var (
		goversion = flag.String("goversion", "", `Go version to build with (will be included in the source package)`)
		cachedir  = flag.String("cachedir", "./build/cache", `Filesystem path to cache the downloaded Go bundles at`)
		signer    = flag.String("signer", "", `Signing key name, also used as package author`)
		upload    = flag.String("upload", "", `Where to upload the source package (usually "ethereum/ethereum")`)
		sshUser   = flag.String("sftp-user", "", `Username for SFTP upload (usually "geth-ci")`)
		workdir   = flag.String("workdir", "", `Output directory for packages (uses temp dir if unset)`)
		now       = time.Now()
	)
	flag.CommandLine.Parse(cmdline)
	*workdir = makeWorkdir(*workdir)
	env := build.Env()
	maybeSkipArchive(env)

	
	if key := getenvBase64("PPA_SIGNING_KEY"); len(key) > 0 {
		gpg := exec.Command("gpg", "--import")
		gpg.Stdin = bytes.NewReader(key)
		build.MustRun(gpg)
	}

	
	gobundle := downloadGoSources(*goversion, *cachedir)

	
	srcdepfetch := goTool("install", "-n", "./...")
	srcdepfetch.Env = append(os.Environ(), "GOPATH="+filepath.Join(*workdir, "modgopath"))
	build.MustRun(srcdepfetch)

	cidepfetch := goTool("run", "./build/ci.go")
	cidepfetch.Env = append(os.Environ(), "GOPATH="+filepath.Join(*workdir, "modgopath"))
	cidepfetch.Run() 

	
	for _, pkg := range debPackages {
		for distro, goboot := range debDistroGoBoots {
			
			meta := newDebMetadata(distro, goboot, *signer, env, now, pkg.Name, pkg.Version, pkg.Executables)
			pkgdir := stageDebianSource(*workdir, meta)

			
			if err := build.ExtractTarballArchive(gobundle, pkgdir); err != nil {
				log.Fatalf("Failed to extract Go sources: %v", err)
			}
			if err := os.Rename(filepath.Join(pkgdir, "go"), filepath.Join(pkgdir, ".go")); err != nil {
				log.Fatalf("Failed to rename Go source folder: %v", err)
			}
			
			os.MkdirAll(filepath.Join(pkgdir, ".mod", "cache"), 0755)
			if err := cp.CopyAll(filepath.Join(pkgdir, ".mod", "cache", "download"), filepath.Join(*workdir, "modgopath", "pkg", "mod", "cache", "download")); err != nil {
				log.Fatalf("Failed to copy Go module dependencies: %v", err)
			}
			
			debuild := exec.Command("debuild", "-S", "-sa", "-us", "-uc", "-d", "-Zxz", "-nc")
			debuild.Dir = pkgdir
			build.MustRun(debuild)

			var (
				basename = fmt.Sprintf("%s_%s", meta.Name(), meta.VersionString())
				source   = filepath.Join(*workdir, basename+".tar.xz")
				dsc      = filepath.Join(*workdir, basename+".dsc")
				changes  = filepath.Join(*workdir, basename+"_source.changes")
			)
			if *signer != "" {
				build.MustRunCommand("debsign", changes)
			}
			if *upload != "" {
				ppaUpload(*workdir, *upload, *sshUser, []string{source, dsc, changes})
			}
		}
	}
}

func downloadGoSources(version string, cachedir string) string {
	csdb := build.MustLoadChecksums("build/checksums.txt")
	file := fmt.Sprintf("go%s.src.tar.gz", version)
	url := "https:
	dst := filepath.Join(cachedir, file)
	if err := csdb.DownloadFile(url, dst); err != nil {
		log.Fatal(err)
	}
	return dst
}

func ppaUpload(workdir, ppa, sshUser string, files []string) {
	p := strings.Split(ppa, "/")
	if len(p) != 2 {
		log.Fatal("-upload PPA name must contain single /")
	}
	if sshUser == "" {
		sshUser = p[0]
	}
	incomingDir := fmt.Sprintf("~%s/ubuntu/%s", p[0], p[1])
	
	var idfile string
	if sshkey := getenvBase64("PPA_SSH_KEY"); len(sshkey) > 0 {
		idfile = filepath.Join(workdir, "sshkey")
		if _, err := os.Stat(idfile); os.IsNotExist(err) {
			ioutil.WriteFile(idfile, sshkey, 0600)
		}
	}
	
	dest := sshUser + "@ppa.launchpad.net"
	if err := build.UploadSFTP(idfile, dest, incomingDir, files); err != nil {
		log.Fatal(err)
	}
}

func getenvBase64(variable string) []byte {
	dec, err := base64.StdEncoding.DecodeString(os.Getenv(variable))
	if err != nil {
		log.Fatal("invalid base64 " + variable)
	}
	return []byte(dec)
}

func makeWorkdir(wdflag string) string {
	var err error
	if wdflag != "" {
		err = os.MkdirAll(wdflag, 0744)
	} else {
		wdflag, err = ioutil.TempDir("", "geth-build-")
	}
	if err != nil {
		log.Fatal(err)
	}
	return wdflag
}

func isUnstableBuild(env build.Environment) bool {
	if env.Tag != "" {
		return false
	}
	return true
}

type debPackage struct {
	Name        string          
	Version     string          
	Executables []debExecutable 
}

type debMetadata struct {
	Env           build.Environment
	GoBootPackage string
	GoBootPath    string

	PackageName string

	
	
	
	Version string

	Author       string 
	Distro, Time string
	Executables  []debExecutable
}

type debExecutable struct {
	PackageName string
	BinaryName  string
	Description string
}



func (d debExecutable) Package() string {
	if d.PackageName != "" {
		return d.PackageName
	}
	return d.BinaryName
}

func newDebMetadata(distro, goboot, author string, env build.Environment, t time.Time, name string, version string, exes []debExecutable) debMetadata {
	if author == "" {
		
		author = "Ethereum Builds <fjl@ethereum.org>"
	}
	return debMetadata{
		GoBootPackage: goboot,
		GoBootPath:    debGoBootPaths[goboot],
		PackageName:   name,
		Env:           env,
		Author:        author,
		Distro:        distro,
		Version:       version,
		Time:          t.Format(time.RFC1123Z),
		Executables:   exes,
	}
}



func (meta debMetadata) Name() string {
	if isUnstableBuild(meta.Env) {
		return meta.PackageName + "-unstable"
	}
	return meta.PackageName
}


func (meta debMetadata) VersionString() string {
	vsn := meta.Version
	if meta.Env.Buildnum != "" {
		vsn += "+build" + meta.Env.Buildnum
	}
	if meta.Distro != "" {
		vsn += "+" + meta.Distro
	}
	return vsn
}


func (meta debMetadata) ExeList() string {
	names := make([]string, len(meta.Executables))
	for i, e := range meta.Executables {
		names[i] = meta.ExeName(e)
	}
	return strings.Join(names, ", ")
}


func (meta debMetadata) ExeName(exe debExecutable) string {
	if isUnstableBuild(meta.Env) {
		return exe.Package() + "-unstable"
	}
	return exe.Package()
}



func (meta debMetadata) ExeConflicts(exe debExecutable) string {
	if isUnstableBuild(meta.Env) {
		
		
		
		
		
		
		
		
		return "ethereum, " + exe.Package()
	}
	return ""
}

func stageDebianSource(tmpdir string, meta debMetadata) (pkgdir string) {
	pkg := meta.Name() + "-" + meta.VersionString()
	pkgdir = filepath.Join(tmpdir, pkg)
	if err := os.Mkdir(pkgdir, 0755); err != nil {
		log.Fatal(err)
	}
	
	build.MustRunCommand("git", "checkout-index", "-a", "--prefix", pkgdir+string(filepath.Separator))

	
	debian := filepath.Join(pkgdir, "debian")
	build.Render("build/deb/"+meta.PackageName+"/deb.rules", filepath.Join(debian, "rules"), 0755, meta)
	build.Render("build/deb/"+meta.PackageName+"/deb.changelog", filepath.Join(debian, "changelog"), 0644, meta)
	build.Render("build/deb/"+meta.PackageName+"/deb.control", filepath.Join(debian, "control"), 0644, meta)
	build.Render("build/deb/"+meta.PackageName+"/deb.copyright", filepath.Join(debian, "copyright"), 0644, meta)
	build.RenderString("8\n", filepath.Join(debian, "compat"), 0644, meta)
	build.RenderString("3.0 (native)\n", filepath.Join(debian, "source/format"), 0644, meta)
	for _, exe := range meta.Executables {
		install := filepath.Join(debian, meta.ExeName(exe)+".install")
		docs := filepath.Join(debian, meta.ExeName(exe)+".docs")
		build.Render("build/deb/"+meta.PackageName+"/deb.install", install, 0644, exe)
		build.Render("build/deb/"+meta.PackageName+"/deb.docs", docs, 0644, exe)
	}
	return pkgdir
}


func doWindowsInstaller(cmdline []string) {
	
	var (
		arch    = flag.String("arch", runtime.GOARCH, "Architecture for cross build packaging")
		signer  = flag.String("signer", "", `Environment variable holding the signing key (e.g. WINDOWS_SIGNING_KEY)`)
		upload  = flag.String("upload", "", `Destination to upload the archives (usually "gethstore/builds")`)
		workdir = flag.String("workdir", "", `Output directory for packages (uses temp dir if unset)`)
	)
	flag.CommandLine.Parse(cmdline)
	*workdir = makeWorkdir(*workdir)
	env := build.Env()
	maybeSkipArchive(env)

	
	var (
		devTools []string
		allTools []string
		gethTool string
	)
	for _, file := range allToolsArchiveFiles {
		if file == "COPYING" { 
			continue
		}
		allTools = append(allTools, filepath.Base(file))
		if filepath.Base(file) == "geth.exe" {
			gethTool = file
		} else {
			devTools = append(devTools, file)
		}
	}

	
	
	templateData := map[string]interface{}{
		"License":  "COPYING",
		"Geth":     gethTool,
		"DevTools": devTools,
	}
	build.Render("build/nsis.geth.nsi", filepath.Join(*workdir, "geth.nsi"), 0644, nil)
	build.Render("build/nsis.install.nsh", filepath.Join(*workdir, "install.nsh"), 0644, templateData)
	build.Render("build/nsis.uninstall.nsh", filepath.Join(*workdir, "uninstall.nsh"), 0644, allTools)
	build.Render("build/nsis.pathupdate.nsh", filepath.Join(*workdir, "PathUpdate.nsh"), 0644, nil)
	build.Render("build/nsis.envvarupdate.nsh", filepath.Join(*workdir, "EnvVarUpdate.nsh"), 0644, nil)
	if err := cp.CopyFile(filepath.Join(*workdir, "SimpleFC.dll"), "build/nsis.simplefc.dll"); err != nil {
		log.Fatal("Failed to copy SimpleFC.dll: %v", err)
	}
	if err := cp.CopyFile(filepath.Join(*workdir, "COPYING"), "COPYING"); err != nil {
		log.Fatal("Failed to copy copyright note: %v", err)
	}
	
	
	
	version := strings.Split(params.Version, ".")
	if env.Commit != "" {
		version[2] += "-" + env.Commit[:8]
	}
	installer, _ := filepath.Abs("geth-" + archiveBasename(*arch, params.ArchiveVersion(env.Commit)) + ".exe")
	build.MustRunCommand("makensis.exe",
		"/DOUTPUTFILE="+installer,
		"/DMAJORVERSION="+version[0],
		"/DMINORVERSION="+version[1],
		"/DBUILDVERSION="+version[2],
		"/DARCH="+*arch,
		filepath.Join(*workdir, "geth.nsi"),
	)
	
	if err := archiveUpload(installer, *upload, *signer); err != nil {
		log.Fatal(err)
	}
}



func doAndroidArchive(cmdline []string) {
	var (
		local  = flag.Bool("local", false, `Flag whether we're only doing a local build (skip Maven artifacts)`)
		signer = flag.String("signer", "", `Environment variable holding the signing key (e.g. ANDROID_SIGNING_KEY)`)
		deploy = flag.String("deploy", "", `Destination to deploy the archive (usually "https:
		upload = flag.String("upload", "", `Destination to upload the archive (usually "gethstore/builds")`)
	)
	flag.CommandLine.Parse(cmdline)
	env := build.Env()

	
	if os.Getenv("ANDROID_HOME") == "" {
		log.Fatal("Please ensure ANDROID_HOME points to your Android SDK")
	}
	
	build.MustRun(goTool("get", "golang.org/x/mobile/cmd/gomobile", "golang.org/x/mobile/cmd/gobind"))
	build.MustRun(gomobileTool("bind", "-ldflags", "-s -w", "--target", "android", "--javapkg", "org.ethereum", "-v", "github.com/ethereum/go-ethereum/mobile"))

	if *local {
		
		os.Rename("geth.aar", filepath.Join(GOBIN, "geth.aar"))
		os.Rename("geth-sources.jar", filepath.Join(GOBIN, "geth-sources.jar"))
		return
	}
	meta := newMavenMetadata(env)
	build.Render("build/mvn.pom", meta.Package+".pom", 0755, meta)

	
	maybeSkipArchive(env)

	
	archive := "geth-" + archiveBasename("android", params.ArchiveVersion(env.Commit)) + ".aar"
	os.Rename("geth.aar", archive)

	if err := archiveUpload(archive, *upload, *signer); err != nil {
		log.Fatal(err)
	}
	
	os.Rename(archive, meta.Package+".aar")
	if *signer != "" && *deploy != "" {
		
		key := getenvBase64(*signer)
		gpg := exec.Command("gpg", "--import")
		gpg.Stdin = bytes.NewReader(key)
		build.MustRun(gpg)
		keyID, err := build.PGPKeyID(string(key))
		if err != nil {
			log.Fatal(err)
		}
		
		repo := *deploy + "/service/local/staging/deploy/maven2"
		if meta.Develop {
			repo = *deploy + "/content/repositories/snapshots"
		}
		build.MustRunCommand("mvn", "gpg:sign-and-deploy-file", "-e", "-X",
			"-settings=build/mvn.settings", "-Durl="+repo, "-DrepositoryId=ossrh",
			"-Dgpg.keyname="+keyID,
			"-DpomFile="+meta.Package+".pom", "-Dfile="+meta.Package+".aar")
	}
}

func gomobileTool(subcmd string, args ...string) *exec.Cmd {
	cmd := exec.Command(filepath.Join(GOBIN, "gomobile"), subcmd)
	cmd.Args = append(cmd.Args, args...)
	cmd.Env = []string{
		"PATH=" + GOBIN + string(os.PathListSeparator) + os.Getenv("PATH"),
	}
	for _, e := range os.Environ() {
		if strings.HasPrefix(e, "GOPATH=") || strings.HasPrefix(e, "PATH=") || strings.HasPrefix(e, "GOBIN=") {
			continue
		}
		cmd.Env = append(cmd.Env, e)
	}
	cmd.Env = append(cmd.Env, "GOBIN="+GOBIN)
	return cmd
}

type mavenMetadata struct {
	Version      string
	Package      string
	Develop      bool
	Contributors []mavenContributor
}

type mavenContributor struct {
	Name  string
	Email string
}

func newMavenMetadata(env build.Environment) mavenMetadata {
	
	contribs := []mavenContributor{}
	if authors, err := os.Open("AUTHORS"); err == nil {
		defer authors.Close()

		scanner := bufio.NewScanner(authors)
		for scanner.Scan() {
			
			line := strings.TrimSpace(scanner.Text())
			if line == "" || line[0] == '#' {
				continue
			}
			
			re := regexp.MustCompile("([^<]+) <(.+)>")
			parts := re.FindStringSubmatch(line)
			if len(parts) == 3 {
				contribs = append(contribs, mavenContributor{Name: parts[1], Email: parts[2]})
			}
		}
	}
	
	version := params.Version
	if isUnstableBuild(env) {
		version += "-SNAPSHOT"
	}
	return mavenMetadata{
		Version:      version,
		Package:      "geth-" + version,
		Develop:      isUnstableBuild(env),
		Contributors: contribs,
	}
}



func doXCodeFramework(cmdline []string) {
	var (
		local  = flag.Bool("local", false, `Flag whether we're only doing a local build (skip Maven artifacts)`)
		signer = flag.String("signer", "", `Environment variable holding the signing key (e.g. IOS_SIGNING_KEY)`)
		deploy = flag.String("deploy", "", `Destination to deploy the archive (usually "trunk")`)
		upload = flag.String("upload", "", `Destination to upload the archives (usually "gethstore/builds")`)
	)
	flag.CommandLine.Parse(cmdline)
	env := build.Env()

	
	build.MustRun(goTool("get", "golang.org/x/mobile/cmd/gomobile", "golang.org/x/mobile/cmd/gobind"))
	build.MustRun(gomobileTool("init"))
	bind := gomobileTool("bind", "-ldflags", "-s -w", "--target", "ios", "-v", "github.com/ethereum/go-ethereum/mobile")

	if *local {
		
		bind.Dir = GOBIN
		build.MustRun(bind)
		return
	}
	archive := "geth-" + archiveBasename("ios", params.ArchiveVersion(env.Commit))
	if err := os.Mkdir(archive, os.ModePerm); err != nil {
		log.Fatal(err)
	}
	bind.Dir, _ = filepath.Abs(archive)
	build.MustRun(bind)
	build.MustRunCommand("tar", "-zcvf", archive+".tar.gz", archive)

	
	maybeSkipArchive(env)

	
	if err := archiveUpload(archive+".tar.gz", *upload, *signer); err != nil {
		log.Fatal(err)
	}
	
	if *deploy != "" {
		meta := newPodMetadata(env, archive)
		build.Render("build/pod.podspec", "Geth.podspec", 0755, meta)
		build.MustRunCommand("pod", *deploy, "push", "Geth.podspec", "--allow-warnings", "--verbose")
	}
}

type podMetadata struct {
	Version      string
	Commit       string
	Archive      string
	Contributors []podContributor
}

type podContributor struct {
	Name  string
	Email string
}

func newPodMetadata(env build.Environment, archive string) podMetadata {
	
	contribs := []podContributor{}
	if authors, err := os.Open("AUTHORS"); err == nil {
		defer authors.Close()

		scanner := bufio.NewScanner(authors)
		for scanner.Scan() {
			
			line := strings.TrimSpace(scanner.Text())
			if line == "" || line[0] == '#' {
				continue
			}
			
			re := regexp.MustCompile("([^<]+) <(.+)>")
			parts := re.FindStringSubmatch(line)
			if len(parts) == 3 {
				contribs = append(contribs, podContributor{Name: parts[1], Email: parts[2]})
			}
		}
	}
	version := params.Version
	if isUnstableBuild(env) {
		version += "-unstable." + env.Buildnum
	}
	return podMetadata{
		Archive:      archive,
		Version:      version,
		Commit:       env.Commit,
		Contributors: contribs,
	}
}



func doXgo(cmdline []string) {
	var (
		alltools = flag.Bool("alltools", false, `Flag whether we're building all known tools, or only on in particular`)
	)
	flag.CommandLine.Parse(cmdline)
	env := build.Env()

	
	gogetxgo := goTool("get", "github.com/karalabe/xgo")
	build.MustRun(gogetxgo)

	
	args := append(buildFlags(env), flag.Args()...)

	if *alltools {
		args = append(args, []string{"--dest", GOBIN}...)
		for _, res := range allToolsArchiveFiles {
			if strings.HasPrefix(res, GOBIN) {
				
				args = append(args, "./"+filepath.Join("cmd", filepath.Base(res)))
				xgo := xgoTool(args)
				build.MustRun(xgo)
				args = args[:len(args)-1]
			}
		}
		return
	}
	
	path := args[len(args)-1]
	args = append(args[:len(args)-1], []string{"--dest", GOBIN, path}...)

	xgo := xgoTool(args)
	build.MustRun(xgo)
}

func xgoTool(args []string) *exec.Cmd {
	cmd := exec.Command(filepath.Join(GOBIN, "xgo"), args...)
	cmd.Env = os.Environ()
	cmd.Env = append(cmd.Env, []string{
		"GOBIN=" + GOBIN,
	}...)
	return cmd
}



func doPurge(cmdline []string) {
	var (
		store = flag.String("store", "", `Destination from where to purge archives (usually "gethstore/builds")`)
		limit = flag.Int("days", 30, `Age threshold above which to delete unstable archives`)
	)
	flag.CommandLine.Parse(cmdline)

	if env := build.Env(); !env.IsCronJob {
		log.Printf("skipping because not a cron job")
		os.Exit(0)
	}
	
	auth := build.AzureBlobstoreConfig{
		Account:   strings.Split(*store, "/")[0],
		Token:     os.Getenv("AZURE_BLOBSTORE_TOKEN"),
		Container: strings.SplitN(*store, "/", 2)[1],
	}
	blobs, err := build.AzureBlobstoreList(auth)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("Found %d blobs\n", len(blobs))

	
	for i := 0; i < len(blobs); i++ {
		if !strings.Contains(blobs[i].Name, "unstable") {
			blobs = append(blobs[:i], blobs[i+1:]...)
			i--
		}
	}
	for i := 0; i < len(blobs); i++ {
		for j := i + 1; j < len(blobs); j++ {
			if blobs[i].Properties.LastModified.After(blobs[j].Properties.LastModified) {
				blobs[i], blobs[j] = blobs[j], blobs[i]
			}
		}
	}
	
	for i, blob := range blobs {
		if time.Since(blob.Properties.LastModified) < time.Duration(*limit)*24*time.Hour {
			blobs = blobs[:i]
			break
		}
	}
	fmt.Printf("Deleting %d blobs\n", len(blobs))
	
	if err := build.AzureBlobstoreDelete(auth, blobs); err != nil {
		log.Fatal(err)
	}
}
