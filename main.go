package main

import (
	"encoding/json"
	"html/template"
	"io/ioutil"
	"log"
	"net/http"
	"net/url"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"
)

// TopStories ...
type TopStories struct {
	IDs   []int
	Items []Item
}

// Item ...
type Item struct {
	Index int    `json:"-"`
	Title string `json:"title"`
	Type  string `json:"type"`
	URL   string `json:"url"`
	Host  string `json:"-"`
}

const numberOfItemsToDisplay int = 30
const apiURL string = "https://hacker-news.firebaseio.com/v0/"
const refreshInterval = (1 * time.Hour)

var topStories TopStories // Stores the in-memory cache of the API response.

func main() {
	port := os.Getenv("PORT")
	if port == "" {
		panic("ERROR: No port set")
	}

	go getTopStories() // Initialize cache

	http.HandleFunc("/", index)

	ticker := time.NewTicker(refreshInterval)
	quit := make(chan struct{})

	go func() {
		for {
			select {
			case <-ticker.C:
				getTopStories()

			case <-quit:
				ticker.Stop()
				return
			}
		}
	}()

	log.Fatal(http.ListenAndServe(":"+port, nil))
}

func index(w http.ResponseWriter, r *http.Request) {
	t, err := template.ParseFiles("index.tmpl")
	if err != nil {
		panic(err)
	}

	w.Header().Set("Content-Type", "text/html")
	if err = t.Execute(w, topStories.Items); err != nil {
		panic(err)
	}
}

func getTopStories() {
	topStories.getIDs()
	topStories.getItems(topStories.IDs[:numberOfItemsToDisplay])
}

func (ts *TopStories) getIDs() TopStories {
	r, err := http.Get(apiURL + "topstories.json")
	if err != nil {
		log.Println("ERROR: Unable to access Hacker News API")
		log.Println(err)
	}
	defer r.Body.Close()

	b, err := ioutil.ReadAll(r.Body)
	if err != nil {
		log.Println(err)
	}

	err = json.Unmarshal(b, &ts.IDs)
	if err != nil {
		log.Println(err)
	}

	return *ts
}

func (ts *TopStories) getItems(ids []int) TopStories {
	var is []Item

	ch := make(chan Item, numberOfItemsToDisplay)

	for index, id := range ids {
		var i Item
		i.Index = index

		sid := strconv.Itoa(id)

		go i.fetch(apiURL+"item/"+sid+".json", ch)

		i = <-ch

		is = append(is, i)
	}

	sort.Slice(is, func(i, j int) bool {
		return is[i].Index < is[j].Index
	})

	ts.Items = is

	return *ts
}

func (i *Item) fetch(apiURL string, ch chan<- Item) {
	r, err := http.Get(apiURL)
	if err != nil {
		log.Printf("ERROR: Get Request | %v\n", apiURL)
		log.Println(err)
		return
	}
	defer r.Body.Close()

	if r.StatusCode != http.StatusOK {
		return
	}

	b, err := ioutil.ReadAll(r.Body)
	if err != nil {
		log.Println(err)
		return
	}

	err = json.Unmarshal(b, &i)
	if err != nil {
		log.Println(err)
		return
	}

	u, err := url.Parse(i.URL)
	if err != nil {
		log.Println(err)
		return
	}

	i.Host = strings.Trim(u.Hostname(), "www.") // @TODO: Make more robust

	ch <- *i
}
