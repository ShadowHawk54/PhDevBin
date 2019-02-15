package main

import (
	"os"
	"strings"

	"github.com/op/go-logging"
	"github.com/cloudkucooland/PhDevBin"
	"github.com/cloudkucooland/PhDevBin/http"
	"github.com/urfave/cli"
)

var flags = []cli.Flag{
	cli.StringFlag{
		Name: "database, d", EnvVar: "DATABASE", Value: "qbin:@tcp(localhost)/qbin",
		Usage: "MySQL/MariaDB connection string. It is recommended to pass this parameter as an environment variable."},
	cli.StringFlag{
		Name: "root, r", EnvVar: "ROOT_URL", Value: "https://qbin.phtiv.com:8000",
		Usage: "The path under which the application will be reachable from the internet."},
	cli.BoolFlag{
		Name: "force-root", EnvVar: "FORCE_ROOT",
		Usage: "If this is set, requests that are not on the root URI will be redirected."},
	cli.StringFlag{
		Name: "wordlist", EnvVar: "WORD_LIST", Value: "eff_large_wordlist.txt",
		Usage: "Word list used for random slug generation."},
	cli.StringFlag{
		Name: "https", EnvVar: "HTTPS_LISTEN", Value: ":8000",
		Usage: "HTTPS listen address."},
	cli.BoolFlag{
		Name: "hsts", EnvVar: "HSTS",
		Usage: "Send HSTS header with max-age=31536000 (1 year)."},
	cli.BoolFlag{
		Name: "hsts-preload", EnvVar: "HSTS_PRELOAD",
		Usage: "Send preload directive with the HSTS header. Requires --hsts."},
	cli.BoolFlag{
		Name: "hsts-subdomains", EnvVar: "HSTS_SUBDOMAINS",
		Usage: "Send includeSubDomains directive with the HSTS header. Requires --hsts."},
	cli.StringFlag{
		Name: "frontend-path, p", EnvVar: "FRONTEND_PATH", Value: "./frontend",
		Usage: "Location of the frontend files."},
	cli.BoolFlag{
		Name: "debug", EnvVar: "DEBUG", 
		Usage: "Show (a lot) more output."},
	cli.BoolFlag{
		Name:  "help, h",
		Usage: "Shows this help, then exits."},
}

func main() {
	app := cli.NewApp()

	app.Name = "PhDevBin"
	app.Version = "0.0.1"
	app.Usage = "qbin-based service - for Phtiv-Draw-Tools"
	app.Flags = flags

	app.HideHelp = true
	cli.AppHelpTemplate = strings.Replace(cli.AppHelpTemplate, "GLOBAL OPTIONS:", "OPTIONS:", 1)

	app.Action = run

	app.Run(os.Args)
}

func run(c *cli.Context) error {
	if c.Bool("help") {
		cli.ShowAppHelp(c)
		return nil
	}

	if c.Bool("debug") {
		PhDevBin.SetLogLevel(logging.DEBUG)
	}

	// Load words
	err := PhDevBin.LoadWordsFile(c.String("wordlist"))
	if err != nil {
		PhDevBin.Log.Errorf("Error loading word list from '%s': %s", c.String("wordlist"), err)
	}

	// Connect to database
	err = PhDevBin.Connect(c.String("database"))
	if err != nil {
		PhDevBin.Log.Errorf("Error connecting to database: %s", err)
		panic(err)
	}

	// Serve HTTP
	if c.String("http") != "none" || c.String("https") != "none" {
		hsts := ""
		if c.String("https") == "none" && c.Bool("hsts") {
			PhDevBin.Log.Warning("You are using --hsts without --https. Ignoring and keeping HSTS off.")
		} else if c.Bool("hsts") {
			hsts = "max-age=31536000"
			if c.Bool("hsts-subdomains") {
				hsts += "; includeSubDomains"
			}
			if c.Bool("hsts-preload") {
				hsts += "; preload"
			}
		} else if c.Bool("hsts-subdomains") || c.Bool("hsts-preload") {
			PhDevBin.Log.Warning("You are using --hsts-subdomains or --hsts-preload without --hsts. Ignoring and keeping HSTS off.")
		}

		go PhDevHTTP.StartHTTP(PhDevHTTP.Configuration{
			ListenHTTPS:   c.String("https"),
			FrontendPath:  c.String("frontend-path"),
			Root:          c.String("root"),
			Hsts:          hsts,
		})
	}

	// Sleep
	select {}
}