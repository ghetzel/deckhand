package main

import (
	"io/ioutil"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/ghetzel/diecast"
	"github.com/ghetzel/go-stockutil/executil"
	"github.com/ghetzel/go-stockutil/fileutil"
	"github.com/ghetzel/go-stockutil/httputil"
	"github.com/ghetzel/go-stockutil/log"
	streamdeck "github.com/magicmonkey/go-streamdeck"
	"github.com/radovskyb/watcher"
)

var DeckhandDir = executil.RootOrString(`/etc/deckhand`, `~/.config/deckhand`)
var DeckhandLockFile = `draw.lock`

type UpdateDeckRequest struct {
	Button
	Deck string
	Page string
}

type Deck struct {
	Name    string
	Page    string
	Pages   []*Page
	Rows    int
	Cols    int
	Count   int
	device  *streamdeck.Device
	watcher *watcher.Watcher
}

func NewDeck() (*Deck, error) {
	if device, err := streamdeck.Open(); err == nil {
		var deck = &Deck{
			Name:   `default`,
			device: device,
		}

		device.ButtonPress(func(i int, d *streamdeck.Device, err error) {
			if err == nil {
				if err := deck.trigger(i); err != nil {
					log.Errorf("btn[%d]: %v", i, err)
				}
			}
		})

		switch strings.ToLower(device.GetName()) {
		case `streamdeck (original v2)`:
			deck.Rows = 3
			deck.Cols = 5
		}

		deck.Count = (deck.Rows * deck.Cols)

		device.ClearButtons()

		return deck, deck.Sync()
	} else {
		return nil, err
	}
}

func (self *Deck) Close() error {
	if self.device != nil {
		self.device.Close()
	}

	return nil
}

func (self *Deck) path(filename ...string) string {
	return filepath.Join(append([]string{fileutil.MustExpandUser(DeckhandDir), self.Name}, filename...)...)
}

func (self *Deck) CurrentPage() *Page {
	var currentPage = `_`

	if self.Page != `` {
		currentPage = self.Page
	}

	for _, page := range self.Pages {
		if page.Name == currentPage {
			return page
		}
	}

	return nil
}

func (self *Deck) Render() error {
	if pg := self.CurrentPage(); pg != nil {
		return pg.Render()
	} else {
		return nil
	}
}

func (self *Deck) Sync() error {
	if pagedirs, err := ioutil.ReadDir(self.path()); err == nil {
		var pages []*Page

		for _, pagedir := range pagedirs {
			if pagedir.IsDir() {
				pages = append(pages, &Page{
					Name: pagedir.Name(),
					deck: self,
				})
			}
		}

		self.Pages = pages
		var merr error

		for _, page := range self.Pages {
			log.AppendError(merr, page.Sync())
		}

		if self.watcher == nil {
			self.watcher = watcher.New()
			self.watcher.SetMaxEvents(1)

			go func() {
				for {
					select {
					case <-self.watcher.Event:
						self.Sync()
					case <-self.watcher.Closed:
						return
					}
				}
			}()

			self.watcher.AddRecursive(self.path())
			go self.watcher.Start(66 * time.Millisecond)
		}

		log.Debugf("deck %v: synced page=%v", self.Name, self.Page)
		return merr
	} else {
		return err
	}
}

func (self *Deck) ListenAndServe(address string) error {
	var server = diecast.NewServer(os.Getenv(`UI`))

	server.Get(`/deckhand/v1/`, func(w http.ResponseWriter, req *http.Request) {
		httputil.RespondJSON(w, `ok`)
	})

	server.Get(`/deckhand/v1/decks/`, func(w http.ResponseWriter, req *http.Request) {
		httputil.RespondJSON(w, []*Deck{self})
	})

	server.Get(`/deckhand/v1/decks/:deck/`, func(w http.ResponseWriter, req *http.Request) {
		httputil.RespondJSON(w, self)
	})

	server.Post(`/deckhand/v1/decks/`, func(w http.ResponseWriter, req *http.Request) {
		var ureq UpdateDeckRequest

		if err := httputil.ParseRequest(req, &ureq); err == nil {

		} else {
			httputil.RespondJSON(w, err)
		}
	})

	return server.ListenAndServe(address)
}

func (self *Deck) trigger(btn int) error {
	if pg := self.CurrentPage(); pg != nil {
		return pg.trigger(btn)
	} else {
		return nil
	}
}
