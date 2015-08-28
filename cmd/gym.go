package main

import (
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path"
	"runtime"
	"strings"
	"time"

	"github.com/jawher/mow.cli"
	"github.com/zbindenren/gym"
)

var gitHashString = "<nil>"
var gitDateString = "<nil"
var buildDateString = "<nil>"

func main() {
	numCPU := runtime.NumCPU()
	gymcmd := cli.App("gym", "golang yum mirror")
	debug := gymcmd.Bool(cli.BoolOpt{Name: "d debug", Desc: "show debug messages"})
	meta := gymcmd.Bool(cli.BoolOpt{Name: "m meta", Desc: "sync only meta data"})
	nocolor := gymcmd.Bool(cli.BoolOpt{Name: "n nocolor", Desc: "disable color output"})
	insecure := gymcmd.Bool(cli.BoolOpt{Name: "i insecure", Desc: "do not verify ssl certificates"})
	workers := gymcmd.Int(cli.IntOpt{Name: "w workers", Value: numCPU, Desc: "number of parallel download workers"})
	gymcmd.Action = func() {
	}
	gymcmd.Command("url", "sync repoository form url", func(cmd *cli.Cmd) {
		cmd.Spec = "[--cert --key] [--cacerts] [-f] URL DESTINATION"

		var (
			filter  = cmd.String(cli.StringOpt{Name: "f filter", Desc: "sync only packages with names containing filter string"})
			cert    = cmd.String(cli.StringOpt{Name: "cert", Desc: "spath to ssl certificate"})
			key     = cmd.String(cli.StringOpt{Name: "key", Desc: "spath to ssl certificate key"})
			cacerts = cmd.String(cli.StringOpt{Name: "cacerts", Desc: "comma separated list of ca certificates"})
		)

		var (
			urlString = cmd.String(cli.StringArg{Name: "URL", Value: "", Desc: "remote yum repository url"})
			dest      = cmd.String(cli.StringArg{Name: "DESTINATION", Value: "", Desc: "local destination directory"})
		)

		cmd.Action = func() {
			if *debug {
				gym.Debug()
			}
			if *nocolor {
				gym.NoColor()
			}
			gym.Log.Info("starting sync",
				"version", gitHashString,
				"mode", "url",
				"debug", *debug,
				"nocolor", *nocolor,
				"insecure", *insecure,
				"meta", *meta,
				"workers", *workers,
				"cert", *cert,
				"key", *key,
				"cacerts", *cacerts,
				"filter", *filter,
				"url", *urlString,
				"destination", *dest,
			)
			u, err := url.Parse(*urlString)
			if err != nil {
				gym.Log.Crit("could not parse url '%s'", urlString)
			}
			var t *http.Transport
			if *insecure && u.Scheme == "https" || len(*cert) > 0 && len(*key) > 0 || len(*cacerts) > 0 {
				caCertList := strings.Split(*cacerts, ",")
				t, err = gym.ConfigureTransport(*insecure, *cert, *key, caCertList...)
				if err != nil {
					gym.Log.Crit("could not configure https transport", "err", err)
				}
			}
			r := gym.NewRepo(*dest, *urlString, t)

			gym.Log.Info("start metadata sync", "url", *urlString, "dest", *dest, "workers", *workers)
			if err := r.SyncMeta(); err != nil {
				gym.Log.Crit("metadata sync failed", "err", err)
			}

			if *meta {
				return
			}
			if err := r.Sync(*filter, *workers); err != nil {
				gym.Log.Crit("rpm sync failed", "err", err)
			}
		}
	})
	gymcmd.Command("repo", "sync repoository form yum repository file", func(cmd *cli.Cmd) {

		cmd.Spec = "[([--exclude] [--enabled]) | ([--repoid] [--name])] [--arch] [-f] -r REPOFILE DESTINATION"

		var (
			filter  = cmd.String(cli.StringOpt{Name: "f filter", Desc: "sync only packages with names containing filter string"})
			exclude = cmd.String(cli.StringOpt{Name: "exclude", Desc: "exclude repositories containing this string"})
			enabled = cmd.Bool(cli.BoolOpt{Name: "enabled", Desc: "sync only enabled repositories"})
			arch    = cmd.String(cli.StringOpt{Name: "arch", Value: "x86_64", Desc: "base architecture e.g: x86_64, PPC"})
			release = cmd.String(cli.StringOpt{Name: "r release", Desc: "release version e.g: Server7, 7.1"})
			repoid  = cmd.String(cli.StringOpt{Name: "repoid", Desc: "only sync repository with name repoid"})
			name    = cmd.String(cli.StringOpt{Name: "name", Desc: "use name instead of repoid as directory name"})
		)

		var (
			repo = cmd.String(cli.StringArg{Name: "REPOFILE", Value: "", Desc: "path to the yum repository file"})
			dest = cmd.String(cli.StringArg{Name: "DESTINATION", Value: "", Desc: "local destination directory"})
		)

		cmd.Action = func() {
			if *debug {
				gym.Debug()
			}
			if *nocolor {
				gym.NoColor()
			}
			gym.Log.Info("starting sync",
				"version", gitHashString,
				"mode", "repo",
				"debug", *debug,
				"nocolor", *nocolor,
				"insecure", *insecure,
				"meta", *meta,
				"workers", *workers,
				"exclude", *exclude,
				"enabled", *enabled,
				"filter", *filter,
				"arch", *arch,
				"release", *release,
				"repo", *repo,
				"repoid", *repoid,
				"destination", *dest,
				"name", *name,
			)

			start := time.Now()
			failedRepositories := []string{}
			skippedRepositories := []string{}
			syncedRepositories := []string{}
			gym.Log.Info("parsing repofile", "file", *repo)
			repos, err := gym.NewRepoList(*repo, *dest, *insecure, *release, *arch)
			if err != nil {
				gym.Log.Crit("could not create repolist", "repofile", *repo, "err", err)
			}
		Loop:
			for _, re := range repos {
				if len(*repoid) > 0 && *repoid != re.Name {
					gym.Log.Info("skipping repository", "name", re.Name, "reason", "excluded")
					skippedRepositories = append(skippedRepositories, re.Name)
					continue
				}
				if *enabled && !re.Enabled {
					gym.Log.Info("skipping repository", "name", re.Name, "reason", "not enabled")
					skippedRepositories = append(skippedRepositories, re.Name)
					continue
				}
				if len(*exclude) > 0 {
					excludedRepoList := strings.Split(*exclude, ",")
					for _, excludeString := range excludedRepoList {
						gym.Log.Info(fmt.Sprintf("%v", excludedRepoList))
						if strings.Contains(re.Name, excludeString) {
							skippedRepositories = append(skippedRepositories, re.Name)
							gym.Log.Info("skipping repository", "name", re.Name, "reason", "excluded")
							continue Loop
						}
					}
				}
				if len(*name) > 0 {
					re.Name = *name
					re.LocalPath = path.Join(path.Dir(re.LocalPath), "/", *name)
				}
				gym.Log.Info("matadata sync", "name", re.Name)
				if err := re.SyncMeta(); err != nil {
					failedRepositories = append(failedRepositories, re.Name)
					gym.Log.Error("metadata sync failed", "err", err)
					continue
				}
				if *meta {
					continue
				}
				if err := re.Sync(*filter, *workers); err != nil {
					failedRepositories = append(failedRepositories, re.Name)
					gym.Log.Error("rpm sync failed", "err", err)
					continue
				}
				syncedRepositories = append(syncedRepositories, re.Name)
			}
			gym.Log.Info("finish",
				"duration", time.Since(start),
				"failedRepositories", len(failedRepositories),
				"skippedRepositories", len(skippedRepositories),
				"syncedRepositories", len(syncedRepositories),
			)
			return

		}
	})

	gymcmd.Command("snapshot", "create snapshot of exsiting yum repository", func(cmd *cli.Cmd) {
		cmd.Spec = "[-c] [-l] SOURCE... DESTINATION"
		var (
			link       = cmd.Bool(cli.BoolOpt{Name: "link l", Desc: "create symlinks instead of copy"})
			createRepo = cmd.Bool(cli.BoolOpt{Name: "createrepo c", Desc: "run create repo"})
		)
		var (
			sources = cmd.Strings(cli.StringsArg{Name: "SOURCE", Value: []string{}, Desc: "path to the yum repository file"})
			dest    = cmd.String(cli.StringArg{Name: "DESTINATION", Value: "", Desc: "local destination directory"})
		)
		cmd.Action = func() {
			if *debug {
				gym.Debug()
			}
			if *nocolor {
				gym.NoColor()
			}
			gym.Log.Info("starting snapshot",
				"version", gitHashString,
				"mode", "snapshot",
				"debug", *debug,
				"nocolor", *nocolor,
				"workers", *workers,
				"destination", *dest,
				"createLinks", *link,
				"createrepo", *createRepo,
				"sources", strings.Join(*sources, ", "),
			)
			start := time.Now()
			failedSources := []string{}
			for _, source := range *sources {
				r := gym.NewRepo(source, "", nil)
				if err := r.Snapshot(*dest, *link, *createRepo, *workers); err != nil {
					failedSources = append(failedSources, source)
					gym.Log.Warn("could not create snapshot", "err", err)
				}
			}
			gym.Log.Info("finish",
				"duration", time.Since(start),
				"failedSources", len(failedSources),
			)
		}
	})

	gymcmd.Command("version", "show version info", func(cmd *cli.Cmd) {
		cmd.Spec = "[-d]"
		var (
			detail = cmd.Bool(cli.BoolOpt{Name: "d detail", Desc: "show detail version info"})
		)
		cmd.Action = func() {
			v := fmt.Sprintf("%s (%v", gitHashString, runtime.Version())
			if *detail {
				v = v + fmt.Sprintf(" - build date: %s, commit date: %s", buildDateString, gitDateString)
			}
			v = v + ")"
			fmt.Println(v)
		}
	})

	if err := gymcmd.Run(os.Args); err != nil {
		fmt.Println(err)
	}

}
