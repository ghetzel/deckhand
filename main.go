package main

import (
	"os"
	"path/filepath"
	"time"

	"github.com/ghetzel/cli"
	"github.com/ghetzel/go-stockutil/log"
	_ "github.com/magicmonkey/go-streamdeck/devices"
)

func main() {
	app := cli.NewApp()
	app.Name = `deckhand`
	app.Usage = `A utility for managing and operating the Elgato Stream Deck series of input devices.`
	app.Version = `0.0.1`

	app.Flags = []cli.Flag{
		cli.StringFlag{
			Name:   `log-level, L`,
			Usage:  `Level of log output verbosity`,
			Value:  `debug`,
			EnvVar: `LOGLEVEL`,
		},
		cli.StringFlag{
			Name:   `config-root, d`,
			Usage:  `The directory where Deck configurations are stored.`,
			Value:  DeckhandDir,
			EnvVar: `DECKHAND_DIR`,
		},
		cli.StringFlag{
			Name:   `page, p`,
			Usage:  `The default page to display.`,
			EnvVar: `DECKHAND_STARTPAGE`,
		},
		cli.StringFlag{
			Name:   `address, a`,
			Usage:  `The address to serve the configuration UI and API from.`,
			Value:  `127.0.0.1:17925`,
			EnvVar: `DECKHAND_ADDRESS`,
		},
	}

	app.Before = func(c *cli.Context) error {
		log.SetLevelString(c.String(`log-level`))
		return nil
	}

	app.Action = func(c *cli.Context) {
		DeckhandDir = c.String(`config-root`)

		var deck, err = OpenDeck(filepath.Join(DeckhandDir, `default`, `deck.yaml`))
		log.FatalIf(err)
		defer deck.Close()

		log.Infof("loaded deck %v", deck.Filename())

		go func() {
			log.FatalIf(deck.ListenAndServe(c.String(`address`)))
		}()

		deck.Page = c.String(`page`)

		for range time.NewTicker(125 * time.Millisecond).C {
			if err := deck.Render(); err != nil {
				log.Warning(err)
			}
		}
	}

	app.Run(os.Args)
}

/*
decks/
  {default,[SERIALNUMBER]}/
		pages/
			{_,[PAGENAME]}/
				{00,01,02...14}/
					fill
					image
					color
					text

*/
