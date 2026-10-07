// Publicguard checks tracked publication content; it cannot detect all PII.
package main

import (
	"bytes"
	"crypto/sha256"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
)

func main() {
	staged := flag.Bool("staged", false, "inspect staged content")
	flag.Parse()
	args := []string{"ls-files", "-z"}
	if *staged {
		args = []string{"diff", "--cached", "--name-only", "--diff-filter=ACMR", "-z"}
	}
	listing, err := exec.Command("git", args...).Output()
	if err != nil {
		fmt.Fprintln(os.Stderr, "Unable to list publication files")
		os.Exit(1)
	}
	risky := regexp.MustCompile(`(?i)(^|/)(\.env($|\.)|secrets|credentials|private|local|data|uploads|backups|exports|logs|project-pack|\.project-pack-qa|\.kube|kubeconfig)(/|$)|\.(pem|key|p12|pfx|jks|tfstate|tfvars|log|dump|sqlite3?|db|csv|zip|gz|tar|docx|pdf|xlsx)$`)
	personalPath := regexp.MustCompile(`/(Users|home)/[^/\s]+/`)
	email := regexp.MustCompile(`[A-Za-z0-9._%+\-]+@[A-Za-z0-9.\-]+\.[A-Za-z]{2,}`)
	failed := false
	for _, raw := range bytes.Split(listing, []byte{0}) {
		name := string(raw)
		if name == "" {
			continue
		}
		reject := func(reason string) {
			fmt.Fprintf(os.Stderr, "Publication blocked: %s (%s)\n", name, reason)
			failed = true
		}
		base := filepath.Base(name)
		environmentFile := strings.HasPrefix(base, ".env") && name != ".env.example"
		sqlExport := strings.HasSuffix(strings.ToLower(name), ".sql") && !strings.HasPrefix(name, "db/migrations/")
		if environmentFile || sqlExport || (risky.MatchString(name) && name != ".env.example") {
			reject("restricted path or file type")
			continue
		}
		var content []byte
		if *staged {
			content, err = exec.Command("git", "show", ":"+name).Output()
		} else {
			content, err = os.ReadFile(name)
		}
		if err != nil {
			reject("cannot inspect content")
			continue
		}
		if len(content) > 1024*1024 || bytes.IndexByte(content, 0) >= 0 {
			reject("binary or oversized content needs explicit policy review")
			continue
		}
		if personalPath.Match(content) {
			reject("personal filesystem path")
		}
		approvedNotice := reviewedNotice(name, content)
		for _, address := range email.FindAllString(string(content), -1) {
			domain := strings.ToLower(strings.SplitN(address, "@", 2)[1])
			if !approvedNotice && domain != "example.com" && domain != "example.org" && domain != "example.net" && !strings.HasSuffix(domain, ".invalid") && domain != "users.noreply.github.com" {
				reject("email address outside synthetic/public Git domains")
				break
			}
		}
	}
	if failed {
		os.Exit(1)
	}
	fmt.Println("Publication guard passed; manual PII and asset review is still required.")
}

// Public copyright contacts are required upstream attribution, not merchant PII.
// Only these reviewed, exact licence bytes qualify; edits lose the exception.
func reviewedNotice(name string, content []byte) bool {
	reviewed := map[string]string{
		"third_party/npm_jridgewell_sourcemap-codec.LICENSE": "769d154fbde32a915af110b1123650bc79f4cbe675acc66e005265bf069c6c6c",
		"third_party/npm_nanoid.LICENSE":                     "da4db1480d9beea3483a2eda5c53b22238d0827d57da162b48f122e04d2d9987",
		"third_party/npm_postcss.LICENSE":                    "5be1f3465bba68a626777f984878814aaf35e7ef8e9fd314d469bcf887050fb8",
	}
	expected, ok := reviewed[name]
	return ok && fmt.Sprintf("%x", sha256.Sum256(content)) == expected
}
