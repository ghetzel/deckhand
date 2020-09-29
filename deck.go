package main

//go:generate esc -o static.go -pkg main -modtime 1500000000 -prefix ui ui

import (
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/ghetzel/diecast"
	"github.com/ghetzel/go-defaults"
	"github.com/ghetzel/go-stockutil/executil"
	"github.com/ghetzel/go-stockutil/fileutil"
	"github.com/ghetzel/go-stockutil/httputil"
	"github.com/ghetzel/go-stockutil/log"
	"github.com/ghetzel/go-stockutil/maputil"
	"github.com/ghetzel/sysfact"
	streamdeck "github.com/magicmonkey/go-streamdeck"
	"github.com/radovskyb/watcher"
	"gopkg.in/yaml.v2"
)

var DeckhandDir = executil.RootOrString(`/etc/deckhand`, `~/.config/deckhand`)
var DeckhandLockFile = `draw.lock`
var systemReport map[string]interface{}

type UpdateDeckRequest struct {
	Button
	Deck string
	Page string
}

type Deck struct {
	Name     string
	Page     string            `yaml:"-" default:"default"`
	Pages    map[string]*Page  `yaml:"pages"`
	Rows     int               `yaml:"rows"`
	Cols     int               `yaml:"cols"`
	Helpers  map[string]string `yaml:"helpers"`
	Count    int               `yaml:"-"`
	device   *streamdeck.Device
	watcher  *watcher.Watcher
	filename string
}

func LoadDeck(filename string) (*Deck, error) {
	var deck = new(Deck)
	deck.filename = filename
	return deck, deck.load(filename)
}

func OpenDeck(filename string) (*Deck, error) {
	if deck, err := LoadDeck(filename); err == nil {
		return deck, deck.Open()
	} else {
		return nil, err
	}
}

func (self *Deck) load(filename string) error {
	filename = fileutil.MustExpandUser(filename)

	if data, err := fileutil.ReadAll(filename); err == nil {
		if err := yaml.Unmarshal(data, self); err == nil {
			self.Name = strings.TrimSuffix(filepath.Base(filename), filepath.Ext(filename))

			return nil
		} else {
			return err
		}
	} else {
		return err
	}
}

func (self *Deck) Filename() string {
	return fileutil.MustExpandUser(self.filename)
}

func (self *Deck) Open() error {
	if device, err := streamdeck.Open(); err == nil {
		switch self.Name {
		case ``, `default`:
			self.Name = `default`
		}

		self.device = device

		self.device.ButtonPress(func(i int, d *streamdeck.Device, err error) {
			if err == nil {
				if err := self.trigger(i); err != nil {
					log.Errorf("btn[%d]: %v", i, err)
				}
			}
		})

		switch strings.ToLower(device.GetName()) {
		case `streamdeck (original v2)`:
			self.Rows = 3
			self.Cols = 5
		}

		self.Count = (self.Rows * self.Cols)

		device.ClearButtons()

		return self.Sync()
	} else {
		return err
	}
}

func (self *Deck) Clear() error {
	self.device.ClearButtons()
	return nil
}

func (self *Deck) Sync() error {
	if err := self.load(self.filename); err != nil {
		return err
	}

	defaults.SetDefaults(self)

	for name, pg := range self.Pages {
		pg.deck = self
		pg.Name = name

		if pg.Name == self.Page {
			self.Clear()

			if err := pg.Sync(); err != nil {
				return fmt.Errorf("page %v: %v", name, err)
			}
		}
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

		self.watcher.Add(self.Filename())
		go self.watcher.Start(250 * time.Millisecond)

		go func() {
			for range time.NewTicker(1000 * time.Millisecond).C {
				if sysreport, err := sysfact.Report(); err == nil {
					systemReport, _ = maputil.DiffuseMap(sysreport, `.`)
				}
			}
		}()
	}

	return nil
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
	var currentPage = `default`

	if self.Page != `` {
		currentPage = self.Page
	}

	if pg, ok := self.Pages[currentPage]; ok {
		return pg
	} else {
		return nil
	}
}

func (self *Deck) Render() error {
	if pg := self.CurrentPage(); pg != nil {
		return pg.Render()
	} else {
		return nil
	}
}

func (self *Deck) ListenAndServe(address string) error {
	var server = diecast.NewServer(os.Getenv(`UI`))

	if dcyml := self.path(`diecast.yml`); fileutil.IsNonemptyFile(dcyml) {
		if err := server.LoadConfig(dcyml); err == nil {
			log.Infof("loaded supplementary config: %v", dcyml)
		} else {
			return err
		}
	}

	if server.RootPath == `` {
		server.SetFileSystem(FS(false))
	}

	server.Get(`/deckhand/v1/`, func(w http.ResponseWriter, req *http.Request) {
		httputil.RespondJSON(w, `ok`)
	})

	server.Get(`/deckhand/v1/report/`, func(w http.ResponseWriter, req *http.Request) {
		httputil.RespondJSON(w, systemReport)
	})

	server.Get(`/deckhand/v1/decks/`, func(w http.ResponseWriter, req *http.Request) {
		httputil.RespondJSON(w, []*Deck{self})
	})

	server.Get(`/deckhand/v1/decks/:deck/`, func(w http.ResponseWriter, req *http.Request) {
		httputil.RespondJSON(w, self)
	})

	server.Get(`/deckhand/v1/decks/:deck/:page/:button/_render/`, func(w http.ResponseWriter, req *http.Request) {
		var page = server.P(req, `page`).String()
		var bidx = int(server.P(req, `button`).Int())

		if pg, ok := self.Pages[page]; ok {
			if btn, ok := pg.Buttons[bidx]; ok {
				w.Header().Set(`Content-Type`, `image/png`)
				btn.RenderTo(w)
			} else {
				httputil.RespondJSON(w, fmt.Errorf("no button %d", bidx), http.StatusNotFound)
			}
		} else {
			httputil.RespondJSON(w, fmt.Errorf("no such page %q", page), http.StatusNotFound)
		}
	})

	server.Get(`/deckhand/v1/decks/:deck/:page/:button/:property/`, func(w http.ResponseWriter, req *http.Request) {
		var page = server.P(req, `page`).String()
		var bidx = int(server.P(req, `button`).Int())
		var prop = server.P(req, `property`).String()

		if pg, ok := self.Pages[page]; ok {
			if btn, ok := pg.Buttons[bidx]; ok {
				btn.ServeProperty(w, req, prop)
			} else {
				httputil.RespondJSON(w, fmt.Errorf("no button %d", bidx), http.StatusNotFound)
			}
		} else {
			httputil.RespondJSON(w, fmt.Errorf("no such page %q", page), http.StatusNotFound)
		}
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
		return pg.trigger(btn + 1)
	} else {
		return nil
	}
}
