package main

import (
	"github.com/rs/zerolog/log"

	"github.com/dm0xa/isola/backend/internal/app"
)

func main() {
	if err := app.Run(app.Options{StartupMessage: "starting isola backend"}); err != nil {
		log.Fatal().Err(err).Msg("server failed")
	}
}
