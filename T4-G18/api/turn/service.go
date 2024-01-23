package turn

import (
	"archive/zip"
	"database/sql"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"time"

	"github.com/alarmfox/game-repository/api"
	"github.com/alarmfox/game-repository/model"
	"gorm.io/gorm"
)

type Repository struct {
	db      *gorm.DB
	dataDir string
}

func NewRepository(db *gorm.DB, dataDir string) *Repository {
	return &Repository{
		db:      db,
		dataDir: dataDir,
	}
}

func (tr *Repository) CreateBulk(r *CreateRequest) ([]Turn, error) {
	turns := make([]model.Turn, len(r.Players))

	err := tr.db.Transaction(func(tx *gorm.DB) error {
		var (
			err error
		)

		//rimosso codice riguardante round
		/* err = tx.Where(&model.Round{ID: r.RoundId}).
			First(&model.Round{}).
			Error
		if err != nil {
			return err
		} */
		
		// controlla se game con quell'id esiste
		err = tx.Where(&model.Game{ID: r.GameID}).
			First(&model.Game{}).
			Error
		if err != nil {
			return err
		}

		var ids []int64
		err = tx.
			Model(&model.Player{}).
			Select("id").
			Where("account_id in ?", r.Players).
			Find(&ids).
			Error

		if err != nil {
			return err
		}

		if len(ids) != len(r.Players) && !api.Duplicated(r.Players) {
			return fmt.Errorf("%w: invalid player list", api.ErrInvalidParam)
		}

		for i, id := range ids {
			turns[i] = model.Turn{
				PlayerID:  id,
				// rimosso RoundID:   r.RoundId,
				GameID:    r.GameID, // aggiunto
				StartedAt: r.StartedAt, 
				ClosedAt:  r.ClosedAt,
			}
		}

		return tx.Create(&turns).Error
	})
	resp := make([]Turn, len(turns))
	for i, turn := range turns {
		resp[i] = fromModel(&turn)
	}

	return resp, api.MakeServiceError(err)
}

func (tr *Repository) Update(id int64, r *UpdateRequest) (Turn, error) {

	var (
		turn model.Turn = model.Turn{ID: id}
		err  error
	)

	err = tr.db.Model(&turn).Updates(r).Error

	return fromModel(&turn), api.MakeServiceError(err)
}

func (tr *Repository) FindById(id int64) (Turn, error) {
	var turn model.Turn

	err := tr.db.
		First(&turn, id).
		Error

	return fromModel(&turn), api.MakeServiceError(err)
}

/*	rimosso perche ovviamente non si può piu cercare per round ma per game
 	func (tr *Repository) FindByRound(id int64) ([]Turn, error) {
	var turns []model.Turn

	err := tr.db.
		Where(&model.Turn{RoundID: id}).
		Find(&turns).
		Error
	resp := make([]Turn, len(turns))
	for i, turn := range turns {
		resp[i] = fromModel(&turn)
	}
	return resp, api.MakeServiceError(err)
} */

// aggiunta funzione findByGame, fa la stessa cosa che faceva findByRound
func (tr *Repository) FindByGame(id int64) ([]Turn, error) {
	var turns []model.Turn

	err := tr.db.
		Where(&model.Turn{GameID: id}).
		Find(&turns).
		Error
	resp := make([]Turn, len(turns))
	for i, turn := range turns {
		resp[i] = fromModel(&turn)
	}
	return resp, api.MakeServiceError(err)
}

func (tr *Repository) Delete(id int64) error {

	db := tr.db.
		Where(&model.Turn{ID: id}).
		Delete(&model.Turn{})

	if db.Error != nil {
		return db.Error
	} else if db.RowsAffected < 1 {
		return api.ErrNotFound
	}

	return nil

}

func (ts *Repository) SaveFile(id int64, r io.Reader) error {
	if r == nil {
		return fmt.Errorf("%w: body is empty", api.ErrInvalidParam)
	}

	err := ts.db.Transaction(func(tx *gorm.DB) error {

		var (
			err  error
			game model.Game
		)

		err = tx.
			Joins("join turns on turns.game_id = game.id where turns.id  = ?", id).
			First(&game).
			Error

		if err != nil {
			return err
		}

		// Crea un file temporaneo per salvare i dati
		dst, err := os.CreateTemp("", "")
		if err != nil {
			return err
		}

		defer os.Remove(dst.Name())

		// Copia i dati nel file temporaneo
		if _, err := io.Copy(dst, r); err != nil {
			return err
		}

		// Verifica che il file sia un archivio zip
		if zfile, err := zip.OpenReader(dst.Name()); err != nil {
			return api.ErrNotAZip
		} else {
			zfile.Close()
		}

		year := time.Now().Year()

		// Definisce il percorso del file finale
		fname := path.Join(ts.dataDir,
			strconv.FormatInt(int64(year), 10),
			strconv.FormatInt(game.ID, 10),
			fmt.Sprintf("%d.zip", id),
		)

		// Crea la directory se non esiste
		dir := path.Dir(fname)
		if err := os.MkdirAll(dir, os.ModePerm); err != nil && !errors.Is(err, os.ErrExist) {
			return err
		}

		// Rinomina il file temporaneo nel percorso finale
		if err := os.Rename(dst.Name(), fname); err != nil {
			return err
		}

		// Crea o aggiorna i metadati per il turno
		return tx.FirstOrCreate(
			&model.Metadata{
				TurnID: sql.NullInt64{Int64: id, Valid: true},
				Path:   fname,
			}).
			Error

	})

	return api.MakeServiceError(err)

}

/* altra implementazione
func (ts *Repository) SaveFile(id int64, r io.Reader) error {
	if r == nil {
		return fmt.Errorf("%w: body is empty", api.ErrInvalidParam)
	}

	err := ts.db.Transaction(func(tx *gorm.DB) error {
		var turn model.Turn // aggiunto

		 rimosso codice associato a round
				var (
		   			err   error
		   			round model.Round
		   		)

		   		err = tx.
		   			Joins("join turns on turns.round_id = rounds.id where turns.id  = ?", id).
		   			First(&round).
		   			Error

		   		if err != nil {
		   			return err
		   		}

		// aggiunto Recupera il turno per ottenere l'ID del game associato
		err := tx.Where("id = ?", id).First(&turn).Error
		if err != nil {
			return err
		}

		// Crea un file temporaneo per salvare i dati
		dst, err := os.CreateTemp("", "")
		if err != nil {
			return err
		}

		defer os.Remove(dst.Name())

		// Copia i dati nel file temporaneo
		if _, err := io.Copy(dst, r); err != nil {
			return err
		}

		// Verifica che il file sia un archivio zip
		if zfile, err := zip.OpenReader(dst.Name()); err != nil {
			return api.ErrNotAZip
		} else {
			zfile.Close()
		}

		year := time.Now().Year()

		// Definisce il percorso del file finale
		fname := path.Join(ts.dataDir,
			strconv.FormatInt(int64(year), 10),
			strconv.FormatInt(turn.GameID, 10),
			fmt.Sprintf("%d.zip", id),
		)

		 		rimosso codice riguardante round
		   		fname := path.Join(ts.dataDir,
		   			strconv.FormatInt(int64(year), 10),
		   			/strconv.FormatInt(round.GameID, 10),
		   			fmt.Sprintf("%d.zip", id),
		   		)

		// Crea la directory se non esiste
		dir := path.Dir(fname)
		if err := os.MkdirAll(dir, os.ModePerm); err != nil && !errors.Is(err, os.ErrExist) {
			return err
		}

		// Rinomina il file temporaneo nel percorso finale
		if err := os.Rename(dst.Name(), fname); err != nil {
			return err
		}

		// Crea o aggiorna i metadati per il turno
		return tx.FirstOrCreate(
			&model.Metadata{
				TurnID: sql.NullInt64{Int64: id, Valid: true},
				Path:   fname,
			}).
			Error

	})

	return api.MakeServiceError(err)

} */

func (ts *Repository) GetFile(id int64) (string, *os.File, error) {
	var (
		metadata model.Metadata
		err      error
	)

	err = ts.db.
		Where(&model.Metadata{TurnID: sql.NullInt64{Int64: id, Valid: true}}).
		First(&metadata).
		Error

	if err != nil {
		return "", nil, api.MakeServiceError(err)
	}

	f, err := os.Open(metadata.Path)

	if errors.Is(err, os.ErrNotExist) {
		return "", nil, api.ErrNotFound
	} else if err != nil {
		return "", nil, err
	}

	return filepath.Base(metadata.Path), f, nil
}
