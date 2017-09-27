package main

import (
	"encoding/json"
	"html/template"
	"io/ioutil"
	"log"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"sort"
	"strconv"
	"sync"
	"time"
)

// Store ...
type Store struct {
	mu    sync.RWMutex
	ids   []int
	items []Item
}

// Item ...
type Item struct {
	Index int    `json:"-"`
	Title string `json:"title"`
	Text  string `json:"text"`
	URL   string `json:"url"`
	Host  string `json:"-"`
}

const numberOfItemsToDisplay int = 30
const apiURL string = "https://hacker-news.firebaseio.com/v0/"
const refreshInterval = (1 * time.Hour)

var (
	env  = os.Getenv("ENV")
	port = os.Getenv("PORT")

	indexTmpl = template.Must(template.ParseFiles("index.tmpl"))
	store     = newStore()
)

func main() {
	if port == "" {
		panic("ERROR: No port set")
	}

	go store.getTopStories() // Initial population of stories

	mux := http.NewServeMux()
	mux.HandleFunc("/", redirectToHTTPS(index))

	hs := &http.Server{
		Addr:         ":" + port,
		Handler:      mux,
		WriteTimeout: 15 * time.Second,
		ReadTimeout:  15 * time.Second,
	}

	ticker := time.NewTicker(refreshInterval)
	quit := make(chan struct{})

	go func() {
		for {
			select {
			case <-ticker.C:
				store.getTopStories()

			case <-quit:
				ticker.Stop()
				return
			}
		}
	}()

	err := hs.ListenAndServe()
	if err != nil {
		log.Fatal("Listen and Serve: ", err)
	}
}

func index(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html")

	if err := indexTmpl.Execute(w, store.Items()); err != nil {
		log.Fatal(err)
	}
}

func redirectToHTTPS(fn func(http.ResponseWriter, *http.Request)) func(http.ResponseWriter, *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		if env != "dev" { // Avoid doing this in development means we do not have to set up local certificates.
			if r.Header.Get("X-Forwarded-Proto") != "https" { // Heroku sends this header to identify HTTPS/HTTP
				target := "https://" + r.Host + r.URL.Path

				http.Redirect(w, r, target, http.StatusMovedPermanently)
			}
		}

		fn(w, r)
	}
}

func (s *Store) getTopStories() {
	s.SetIDs(fetchIDs())
	s.SetItems(fetchItems(s.IDs()))
}

func fetchIDs() []int {
	r, err := http.Get(apiURL + "topstories.json")
	if err != nil {
		log.Println("ERROR: Unable to access Hacker News API")
		log.Println(err)
		return []int{}
	}
	defer r.Body.Close()

	var ids []int

	b, err := ioutil.ReadAll(r.Body) // @TODO: Remove ReadAll, too heavy a call
	if err != nil {
		log.Println(err)
	}

	err = json.Unmarshal(b, &ids)
	if err != nil {
		log.Println(err)
	}

	return ids
}

func fetchItems(ids []int) []Item {
	var is []Item // Temporary storage to switch out with real storage
	var count int

	ch := make(chan Item, numberOfItemsToDisplay)

	for index, id := range ids {
		if count == numberOfItemsToDisplay {
			break
		}

		var i Item
		i.Index = index

		sid := strconv.Itoa(id)

		go i.fetch(apiURL+"item/"+sid+".json", ch)

		i = <-ch

		if i.Text != "" { // This story is a discussion link
			continue
		}

		is = append(is, i)

		count = count + 1
	}

	sort.Slice(is, func(i, j int) bool {
		return is[i].Index < is[j].Index
	})

	return is
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

	b, err := ioutil.ReadAll(r.Body) // @TODO: Remove ReadAll, too heavy a call
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

	i.Host = trimSubdomain(u.Hostname())

	ch <- *i
}

func newStore() *Store {
	return &Store{items: []Item{}}
}

func (s *Store) SetIDs(ids []int) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.ids = ids
}

func (s *Store) IDs() []int {
	s.mu.RLock()
	defer s.mu.RUnlock()

	return s.ids
}

func (s *Store) SetItems(items []Item) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.items = items
}

func (s *Store) Items() []Item {
	s.mu.RLock()
	defer s.mu.RUnlock()

	return s.items
}

func trimSubdomain(s string) string {
	re, err := regexp.Compile(`(https?://|w{3}\.)`)
	if err != nil {
		log.Println(err)
		return ""
	}

	return re.ReplaceAllString(s, "")
}
