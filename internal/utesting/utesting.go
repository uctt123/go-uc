




















package utesting

import (
	"bytes"
	"fmt"
	"io"
	"regexp"
	"runtime"
	"sync"
	"time"
)


type Test struct {
	Name string
	Fn   func(*T)
}


type Result struct {
	Name     string
	Failed   bool
	Output   string
	Duration time.Duration
}


func MatchTests(tests []Test, expr string) []Test {
	var results []Test
	re, err := regexp.Compile(expr)
	if err != nil {
		return nil
	}
	for _, test := range tests {
		if re.MatchString(test.Name) {
			results = append(results, test)
		}
	}
	return results
}



func RunTests(tests []Test, report io.Writer) []Result {
	results := make([]Result, len(tests))
	for i, test := range tests {
		var output io.Writer
		buffer := new(bytes.Buffer)
		output = buffer
		if report != nil {
			output = io.MultiWriter(buffer, report)
		}
		start := time.Now()
		results[i].Name = test.Name
		results[i].Failed = run(test, output)
		results[i].Duration = time.Since(start)
		results[i].Output = buffer.String()
		if report != nil {
			printResult(results[i], report)
		}
	}
	return results
}

func printResult(r Result, w io.Writer) {
	pd := r.Duration.Truncate(100 * time.Microsecond)
	if r.Failed {
		fmt.Fprintf(w, "-- FAIL %s (%v)\n", r.Name, pd)
	} else {
		fmt.Fprintf(w, "-- OK %s (%v)\n", r.Name, pd)
	}
}


func CountFailures(rr []Result) int {
	count := 0
	for _, r := range rr {
		if r.Failed {
			count++
		}
	}
	return count
}


func Run(test Test) (bool, string) {
	output := new(bytes.Buffer)
	failed := run(test, output)
	return failed, output.String()
}

func run(test Test, output io.Writer) bool {
	t := &T{output: output}
	done := make(chan struct{})
	go func() {
		defer close(done)
		defer func() {
			if err := recover(); err != nil {
				buf := make([]byte, 4096)
				i := runtime.Stack(buf, false)
				t.Logf("panic: %v\n\n%s", err, buf[:i])
				t.Fail()
			}
		}()
		test.Fn(t)
	}()
	<-done
	return t.failed
}



type T struct {
	mu     sync.Mutex
	failed bool
	output io.Writer
}



func (t *T) FailNow() {
	t.Fail()
	runtime.Goexit()
}


func (t *T) Fail() {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.failed = true
}


func (t *T) Failed() bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.failed
}



func (t *T) Log(vs ...interface{}) {
	t.mu.Lock()
	defer t.mu.Unlock()
	fmt.Fprintln(t.output, vs...)
}



func (t *T) Logf(format string, vs ...interface{}) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if len(format) == 0 || format[len(format)-1] != '\n' {
		format += "\n"
	}
	fmt.Fprintf(t.output, format, vs...)
}


func (t *T) Error(vs ...interface{}) {
	t.Log(vs...)
	t.Fail()
}


func (t *T) Errorf(format string, vs ...interface{}) {
	t.Logf(format, vs...)
	t.Fail()
}


func (t *T) Fatal(vs ...interface{}) {
	t.Log(vs...)
	t.FailNow()
}


func (t *T) Fatalf(format string, vs ...interface{}) {
	t.Logf(format, vs...)
	t.FailNow()
}
