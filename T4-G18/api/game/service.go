package game

import (
	"fmt"
	"time"

	"github.com/alarmfox/game-repository/api"
	"github.com/alarmfox/game-repository/model"
	"gorm.io/gorm"
)

type Repository struct {
	db *gorm.DB
}

func NewRepository(db *gorm.DB) *Repository {
	return &Repository{
		db: db,
	}
}

func (gs *Repository) Create(r *CreateRequest) (Game, error) {
	var (
		game = model.Game{
			Name:       r.Name,
			StartedAt:  r.StartedAt,
			ClosedAt:   r.ClosedAt,
			Class:      r.Class,      // aggiunto
			Robot:      r.Robot,      // aggiunto
			Difficulty: r.Difficulty, // aggiunto
			Players:    make([]model.Player, len(r.Players)),
		}
	)
	// detect duplication in player
	if api.Duplicated(r.Players) {
		return Game{}, api.ErrInvalidParam
	}

	for i, player := range r.Players {
		game.Players[i] = model.Player{
			AccountID: player,
		}
	}

	err := gs.db.Transaction(func(tx *gorm.DB) error {
		return tx.Create(&game).Error
	})

	if err != nil {
		return Game{}, api.MakeServiceError(err)
	}
	game.Players = nil

	return fromModel(&game), nil
}

func (gs *Repository) FindById(id int64) (Game, error) {
	var game model.Game
	err := gs.db.
		Preload("Players").
		First(&game, id).
		Error

	return fromModel(&game), api.MakeServiceError(err)
}

func (gs *Repository) FindByInterval(accountId string, i api.IntervalParams, p api.PaginationParams) ([]Game, int64, error) {
	var (
		games []model.Game
		n     int64
		err   error
	)

	if accountId != "" {
		err = gs.db.Transaction(func(tx *gorm.DB) error {
			association := tx.Model(&model.Player{AccountID: accountId}).
				Scopes(api.WithInterval(i, "games.created_at"),
					api.WithPagination(p)).
				Order("games.created_at desc").
				Association("Games")

			n = association.Count()
			return association.Find(&games)

		})
	} else {
		err = gs.db.Scopes(api.WithInterval(i, "games.created_at"),
			api.WithPagination(p)).
			Find(&games).
			Count(&n).
			Error
	}
	res := make([]Game, len(games))
	for i, game := range games {
		res[i] = fromModel(&game)
	}
	return res, n, api.MakeServiceError(err)
}

func (gs *Repository) Delete(id int64) error {
	db := gs.db.
		Where(&model.Game{ID: id}).
		Delete(&model.Game{})

	if db.Error != nil {
		return api.MakeServiceError(db.Error)
	} else if db.RowsAffected < 1 {
		return api.ErrNotFound
	}
	return nil
}

// funzione modificata
func (gs *Repository) Update(id int64, r *UpdateRequest) (Game, error) {

	var (
		game model.Game = model.Game{ID: id}
		err  error
	)

	// Aggiorna il gioco con i nuovi valori
	err = gs.db.Model(&game).Updates(r).Error
	if err != nil {
		return Game{}, api.MakeServiceError(err)
	}

	// Ricarica il gioco per ottenere i dati aggiornati, inclusi StartedAt e ClosedAt
	err = gs.db.First(&game, id).Error
	if err != nil {
		return Game{}, api.MakeServiceError(err)
	}

	// Controlla se StartedAt che ClosedAt sono non nulli
	if game.StartedAt != nil && game.ClosedAt != nil {
		duration := game.ClosedAt.Sub(*game.StartedAt)
		durationToAddPlayer := game.ClosedAt.Sub(*game.StartedAt)
		// Calcola ore, minuti, secondi
		hours := duration / time.Hour
		duration -= hours * time.Hour
		minutes := duration / time.Minute
		duration -= minutes * time.Minute
		seconds := duration / time.Second

		// Crea una stringa nel formato HH:MM:SS
		durationStr := fmt.Sprintf("%02d:%02d:%02d", hours, minutes, seconds)

		game.Duration = durationStr

		// Aggiorna nuovamente il gioco con la durata calcolata
		err = gs.db.Model(&game).Update("Duration", durationStr).Error
		if err != nil {
			return Game{}, api.MakeServiceError(err)
		}

		// Trova il Round associato al Game
		var round model.Round
		err := gs.db.Where("game_id = ?", id).First(&round).Error
		if err != nil {
			fmt.Println("Errore nella ricerca del Round:", err)
		}

		// Trova il Turn associato al Round
		var turn model.Turn
		err = gs.db.Where("round_id = ?", round.ID).First(&turn).Error
		if err != nil {
			fmt.Println("Errore nella ricerca del Turn:", err)
		}

		// Trova il Player associato al Turn
		var player model.Player
		err = gs.db.First(&player, turn.PlayerID).Error
		if err != nil {
			fmt.Println("Errore nella ricerca del Player:", err)
		}

		// Aggiorna TotalTimePlayed per il Player
		player.TotalTimePlayed += durationToAddPlayer
		err = gs.db.Model(&player).Update("TotalTimePlayed", player.TotalTimePlayed).Error
		if err != nil {
			fmt.Println("Errore nell'aggiornamento di TotalTimePlayed:", err)
		}
		if turn.IsWinner {
			// Crea un oggetto PlayerGame con le chiavi primarie impostates
			playerGame := model.PlayerGame{
				PlayerID: turn.PlayerID,
				GameID:   id,
			}

			// Aggiorna il campo IsWinner a true per l'oggetto specificato
			err = gs.db.Model(&playerGame).Update("IsWinner", true).Error
			if err != nil {
				fmt.Println("Errore nell'aggiornamento di IsWinner in PlayerGame:", err)
			}
		}
	}

	return fromModel(&game), api.MakeServiceError(err)
}
