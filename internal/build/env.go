















package build

import (
	"flag"
	"fmt"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"
)

var (
	
	GitCommitFlag   = flag.String("git-commit", "", `Overrides git commit hash embedded into executables`)
	GitBranchFlag   = flag.String("git-branch", "", `Overrides git branch being built`)
	GitTagFlag      = flag.String("git-tag", "", `Overrides git tag being built`)
	BuildnumFlag    = flag.String("buildnum", "", `Overrides CI build number`)
	PullRequestFlag = flag.Bool("pull-request", false, `Overrides pull request status of the build`)
	CronJobFlag     = flag.Bool("cron-job", false, `Overrides cron job status of the build`)
)


type Environment struct {
	Name                      string 
	Repo                      string 
	Commit, Date, Branch, Tag string 
	Buildnum                  string
	IsPullRequest             bool
	IsCronJob                 bool
}

func (env Environment) String() string {
	return fmt.Sprintf("%s env (commit:%s date:%s branch:%s tag:%s buildnum:%s pr:%t)",
		env.Name, env.Commit, env.Date, env.Branch, env.Tag, env.Buildnum, env.IsPullRequest)
}



func Env() Environment {
	switch {
	case os.Getenv("CI") == "true" && os.Getenv("TRAVIS") == "true":
		commit := os.Getenv("TRAVIS_PULL_REQUEST_SHA")
		if commit == "" {
			commit = os.Getenv("TRAVIS_COMMIT")
		}
		return Environment{
			Name:          "travis",
			Repo:          os.Getenv("TRAVIS_REPO_SLUG"),
			Commit:        commit,
			Date:          getDate(commit),
			Branch:        os.Getenv("TRAVIS_BRANCH"),
			Tag:           os.Getenv("TRAVIS_TAG"),
			Buildnum:      os.Getenv("TRAVIS_BUILD_NUMBER"),
			IsPullRequest: os.Getenv("TRAVIS_PULL_REQUEST") != "false",
			IsCronJob:     os.Getenv("TRAVIS_EVENT_TYPE") == "cron",
		}
	case os.Getenv("CI") == "True" && os.Getenv("APPVEYOR") == "True":
		commit := os.Getenv("APPVEYOR_PULL_REQUEST_HEAD_COMMIT")
		if commit == "" {
			commit = os.Getenv("APPVEYOR_REPO_COMMIT")
		}
		return Environment{
			Name:          "appveyor",
			Repo:          os.Getenv("APPVEYOR_REPO_NAME"),
			Commit:        commit,
			Date:          getDate(commit),
			Branch:        os.Getenv("APPVEYOR_REPO_BRANCH"),
			Tag:           os.Getenv("APPVEYOR_REPO_TAG_NAME"),
			Buildnum:      os.Getenv("APPVEYOR_BUILD_NUMBER"),
			IsPullRequest: os.Getenv("APPVEYOR_PULL_REQUEST_NUMBER") != "",
			IsCronJob:     os.Getenv("APPVEYOR_SCHEDULED_BUILD") == "True",
		}
	default:
		return LocalEnv()
	}
}


func LocalEnv() Environment {
	env := applyEnvFlags(Environment{Name: "local", Repo: "ethereum/go-ethereum"})

	head := readGitFile("HEAD")
	if fields := strings.Fields(head); len(fields) == 2 {
		head = fields[1]
	} else {
		
		
		
		commitRe, _ := regexp.Compile("^([0-9a-f]{40})$")
		if commit := commitRe.FindString(head); commit != "" && env.Commit == "" {
			env.Commit = commit
		}
		return env
	}
	if env.Commit == "" {
		env.Commit = readGitFile(head)
	}
	env.Date = getDate(env.Commit)
	if env.Branch == "" {
		if head != "HEAD" {
			env.Branch = strings.TrimPrefix(head, "refs/heads/")
		}
	}
	if info, err := os.Stat(".git/objects"); err == nil && info.IsDir() && env.Tag == "" {
		env.Tag = firstLine(RunGit("tag", "-l", "--points-at", "HEAD"))
	}
	return env
}

func firstLine(s string) string {
	return strings.Split(s, "\n")[0]
}

func getDate(commit string) string {
	if commit == "" {
		return ""
	}
	out := RunGit("show", "-s", "--format=%ct", commit)
	if out == "" {
		return ""
	}
	date, err := strconv.ParseInt(strings.TrimSpace(out), 10, 64)
	if err != nil {
		panic(fmt.Sprintf("failed to parse git commit date: %v", err))
	}
	return time.Unix(date, 0).Format("20060102")
}

func applyEnvFlags(env Environment) Environment {
	if !flag.Parsed() {
		panic("you need to call flag.Parse before Env or LocalEnv")
	}
	if *GitCommitFlag != "" {
		env.Commit = *GitCommitFlag
	}
	if *GitBranchFlag != "" {
		env.Branch = *GitBranchFlag
	}
	if *GitTagFlag != "" {
		env.Tag = *GitTagFlag
	}
	if *BuildnumFlag != "" {
		env.Buildnum = *BuildnumFlag
	}
	if *PullRequestFlag {
		env.IsPullRequest = true
	}
	if *CronJobFlag {
		env.IsCronJob = true
	}
	return env
}
