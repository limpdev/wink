package main

import (
	"html/template"
	"log"
	"os"

	"gopkg.in/yaml.v3"
)

type Link struct {
	Name   string   `yaml:"name"   json:"name"`
	Href   string   `yaml:"href"   json:"href"`
	Env    string   `yaml:"env,omitempty"    json:"env,omitempty"`
	Desc   string   `yaml:"desc,omitempty"   json:"desc,omitempty"`
	Tags   []string `yaml:"tags,omitempty"   json:"tags,omitempty"`
	Pinned bool     `yaml:"pinned,omitempty" json:"pinned,omitempty"`
}
type Section struct {
	ID    string `yaml:"id"    json:"id"`
	Label string `yaml:"label" json:"label"`
	Tag   string `yaml:"tag"   json:"tag"`
	Links []Link `yaml:"links" json:"links"`
}
type Config struct {
	Title    string    `yaml:"title"    json:"title"`
	Sections []Section `yaml:"sections" json:"sections"`
}
type Heading struct {
	Level   int    `yaml:"level"`
	Content string `yaml:"content"`
	Class   string `yaml:"class,omitempty"`
	ID      string `yaml:"id,omitempty"`
}
type PageData struct {
	Config Config
	CSS    template.CSS
}

func main() {
	configData, err := os.ReadFile("config.yaml")
	onErr(err, "Error reading config.yaml")
	var config Config
	err = yaml.Unmarshal(configData, &config)
	onErr(err, "Error parsing config.yaml")
	cssData, err := os.ReadFile("styles.css")
	onErr(err, "Error reading styles.css")
	tmplData, err := os.ReadFile("aio.html")
	onErr(err, "Error reading template.html")
	tmpl, err := template.New("page").Parse(string(tmplData))
	onErr(err, "Error parsing template")
	outFile, err := os.Create("wink.html")
	onErr(err, "Error creating output file")
	defer outFile.Close()
	pageData := PageData{Config: config, CSS: template.CSS(cssData)}
	err = tmpl.Execute(outFile, pageData)
	onErr(err, "Error executing template")
	log.Println("Successfully generated wink.html")
}

func onErr(err error, msg string) {
	if err != nil {
		log.Fatalf("%s: %v", msg, err)
	}
}
