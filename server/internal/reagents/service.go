package reagents

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

// ---------------------------------------------------------------------------
// Errors
// ---------------------------------------------------------------------------

var (
	ErrNotFound     = errors.New("not found")
	ErrInvalidInput = errors.New("invalid input")
)

// ---------------------------------------------------------------------------
// Types — one struct per reagent entity
// ---------------------------------------------------------------------------

type Storage struct {
	ID           int       `json:"id"`
	Name         string    `json:"name"`
	LocationType string    `json:"locationType"`
	Description  string    `json:"description"`
	CreatedAt    time.Time `json:"createdAt"`
	UpdatedAt    time.Time `json:"updatedAt"`
}

type Box struct {
	ID        int       `json:"id"`
	BoxNo     string    `json:"boxNo"`
	BoxType   string    `json:"boxType"`
	Owner     string    `json:"owner"`
	Label     string    `json:"label"`
	Location  string    `json:"location"`
	Drawer    string    `json:"drawer"`
	Position  string    `json:"position"`
	StorageID *int      `json:"storageId"`
	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
}

type Antibody struct {
	ID           int       `json:"id"`
	AntibodyName string    `json:"antibodyName"`
	CatalogNo    string    `json:"catalogNo"`
	Company      string    `json:"company"`
	Class        string    `json:"class"`
	Antigen      string    `json:"antigen"`
	Host         string    `json:"host"`
	Investigator string    `json:"investigator"`
	ExpID        string    `json:"expId"`
	Notes        string    `json:"notes"`
	BoxID        *int      `json:"boxId"`
	Location     string    `json:"location"`
	Quantity     string    `json:"quantity"`
	IsDepleted   bool      `json:"isDepleted"`
	CreatedAt    time.Time `json:"createdAt"`
	UpdatedAt    time.Time `json:"updatedAt"`
}

type CellLine struct {
	ID           int       `json:"id"`
	CellLineName string    `json:"cellLineName"`
	Selection    string    `json:"selection"`
	Species      string    `json:"species"`
	ParentalCell string    `json:"parentalCell"`
	Medium       string    `json:"medium"`
	ObtainFrom   string    `json:"obtainFrom"`
	CellType     string    `json:"cellType"`
	BoxID        *int      `json:"boxId"`
	Location     string    `json:"location"`
	Owner        string    `json:"owner"`
	Label        string    `json:"label"`
	Notes        string    `json:"notes"`
	IsDepleted   bool      `json:"isDepleted"`
	CreatedAt    time.Time `json:"createdAt"`
	UpdatedAt    time.Time `json:"updatedAt"`
}

type Virus struct {
	ID         int       `json:"id"`
	VirusName  string    `json:"virusName"`
	VirusType  string    `json:"virusType"`
	BoxID      *int      `json:"boxId"`
	Location   string    `json:"location"`
	Owner      string    `json:"owner"`
	Label      string    `json:"label"`
	Notes      string    `json:"notes"`
	IsDepleted bool      `json:"isDepleted"`
	CreatedAt  time.Time `json:"createdAt"`
	UpdatedAt  time.Time `json:"updatedAt"`
}

type DNA struct {
	ID         int       `json:"id"`
	DNAName    string    `json:"dnaName"`
	DNAType    string    `json:"dnaType"`
	BoxID      *int      `json:"boxId"`
	Location   string    `json:"location"`
	Owner      string    `json:"owner"`
	Label      string    `json:"label"`
	Notes      string    `json:"notes"`
	IsDepleted bool      `json:"isDepleted"`
	CreatedAt  time.Time `json:"createdAt"`
	UpdatedAt  time.Time `json:"updatedAt"`
}

type Oligo struct {
	ID         int       `json:"id"`
	OligoName  string    `json:"oligoName"`
	Sequence   string    `json:"sequence"`
	OligoType  string    `json:"oligoType"`
	BoxID      *int      `json:"boxId"`
	Location   string    `json:"location"`
	Owner      string    `json:"owner"`
	Label      string    `json:"label"`
	Notes      string    `json:"notes"`
	IsDepleted bool      `json:"isDepleted"`
	CreatedAt  time.Time `json:"createdAt"`
	UpdatedAt  time.Time `json:"updatedAt"`
}

type Chemical struct {
	ID           int       `json:"id"`
	ChemicalName string    `json:"chemicalName"`
	CatalogNo    string    `json:"catalogNo"`
	Company      string    `json:"company"`
	ChemType     string    `json:"chemType"`
	BoxID        *int      `json:"boxId"`
	Location     string    `json:"location"`
	Owner        string    `json:"owner"`
	Label        string    `json:"label"`
	Notes        string    `json:"notes"`
	IsDepleted   bool      `json:"isDepleted"`
	CreatedAt    time.Time `json:"createdAt"`
	UpdatedAt    time.Time `json:"updatedAt"`
}

type Molecular struct {
	ID         int       `json:"id"`
	MRName     string    `json:"mrName"`
	MRType     string    `json:"mrType"`
	BoxID      *int      `json:"boxId"`
	Location   string    `json:"location"`
	Position   string    `json:"position"`
	Owner      string    `json:"owner"`
	Label      string    `json:"label"`
	Notes      string    `json:"notes"`
	IsDepleted bool      `json:"isDepleted"`
	CreatedAt  time.Time `json:"createdAt"`
	UpdatedAt  time.Time `json:"updatedAt"`
}

type SearchResult struct {
	Type string      `json:"type"`
	ID   int         `json:"id"`
	Name string      `json:"name"`
	Item interface{} `json:"item"`
}

// ---------------------------------------------------------------------------
// Service
// ---------------------------------------------------------------------------

type Service struct {
	db *sql.DB
}

func NewService(db *sql.DB) *Service {
	return &Service{db: db}
}

// ========================== STORAGE ==========================

func (s *Service) ListStorage(ctx context.Context) ([]Storage, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, name, COALESCE(location_type,''), COALESCE(description,''), created_at, updated_at
		 FROM reagent_storage ORDER BY name`)
	if err != nil {
		return nil, fmt.Errorf("list storage: %w", err)
	}
	defer rows.Close()
	var out []Storage
	for rows.Next() {
		var r Storage
		if err := rows.Scan(&r.ID, &r.Name, &r.LocationType, &r.Description, &r.CreatedAt, &r.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	if out == nil {
		out = []Storage{}
	}
	return out, nil
}

func (s *Service) CreateStorage(ctx context.Context, r Storage, userID string) (*Storage, error) {
	if strings.TrimSpace(r.Name) == "" {
		return nil, fmt.Errorf("%w: name is required", ErrInvalidInput)
	}
	err := s.db.QueryRowContext(ctx,
		`INSERT INTO reagent_storage(name, location_type, description, updated_by)
		 VALUES($1,$2,$3,$4) RETURNING id, created_at, updated_at`,
		r.Name, r.LocationType, r.Description, userID,
	).Scan(&r.ID, &r.CreatedAt, &r.UpdatedAt)
	if err != nil {
		return nil, fmt.Errorf("create storage: %w", err)
	}
	return &r, nil
}

func (s *Service) UpdateStorage(ctx context.Context, id int, r Storage, userID string) error {
	res, err := s.db.ExecContext(ctx,
		`UPDATE reagent_storage SET name=$1, location_type=$2, description=$3, updated_by=$4 WHERE id=$5`,
		r.Name, r.LocationType, r.Description, userID, id)
	if err != nil {
		return fmt.Errorf("update storage: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *Service) DeleteStorage(ctx context.Context, id int, userID string) error {
	// Set updated_by before delete so audit trigger captures who deleted
	s.db.ExecContext(ctx, `UPDATE reagent_storage SET updated_by=$1 WHERE id=$2`, userID, id)
	res, err := s.db.ExecContext(ctx, `DELETE FROM reagent_storage WHERE id=$1`, id)
	if err != nil {
		return fmt.Errorf("delete storage: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// ========================== BOXES ==========================

func (s *Service) ListBoxes(ctx context.Context, q string) ([]Box, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, box_no, COALESCE(box_type,''), COALESCE(owner,''), COALESCE(label,''),
		        COALESCE(location,''), COALESCE(drawer,''), COALESCE(position,''),
		        storage_id, created_at, updated_at
		 FROM reagent_box
		 WHERE ($1 = '' OR box_no ILIKE '%' || $1 || '%' OR label ILIKE '%' || $1 || '%')
		 ORDER BY box_no`, q)
	if err != nil {
		return nil, fmt.Errorf("list boxes: %w", err)
	}
	defer rows.Close()
	var out []Box
	for rows.Next() {
		var b Box
		if err := rows.Scan(&b.ID, &b.BoxNo, &b.BoxType, &b.Owner, &b.Label,
			&b.Location, &b.Drawer, &b.Position, &b.StorageID, &b.CreatedAt, &b.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, b)
	}
	if out == nil {
		out = []Box{}
	}
	return out, nil
}

func (s *Service) CreateBox(ctx context.Context, b Box, userID string) (*Box, error) {
	if strings.TrimSpace(b.BoxNo) == "" {
		return nil, fmt.Errorf("%w: boxNo is required", ErrInvalidInput)
	}
	err := s.db.QueryRowContext(ctx,
		`INSERT INTO reagent_box(box_no, box_type, owner, label, location, drawer, position, storage_id, updated_by)
		 VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9) RETURNING id, created_at, updated_at`,
		b.BoxNo, b.BoxType, b.Owner, b.Label, b.Location, b.Drawer, b.Position, b.StorageID, userID,
	).Scan(&b.ID, &b.CreatedAt, &b.UpdatedAt)
	if err != nil {
		return nil, fmt.Errorf("create box: %w", err)
	}
	return &b, nil
}

func (s *Service) UpdateBox(ctx context.Context, id int, b Box, userID string) error {
	res, err := s.db.ExecContext(ctx,
		`UPDATE reagent_box SET box_no=$1, box_type=$2, owner=$3, label=$4, location=$5,
		        drawer=$6, position=$7, storage_id=$8, updated_by=$9 WHERE id=$10`,
		b.BoxNo, b.BoxType, b.Owner, b.Label, b.Location, b.Drawer, b.Position, b.StorageID, userID, id)
	if err != nil {
		return fmt.Errorf("update box: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *Service) DeleteBox(ctx context.Context, id int, userID string) error {
	s.db.ExecContext(ctx, `UPDATE reagent_box SET updated_by=$1 WHERE id=$2`, userID, id)
	res, err := s.db.ExecContext(ctx, `DELETE FROM reagent_box WHERE id=$1`, id)
	if err != nil {
		return fmt.Errorf("delete box: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// ========================== ANTIBODIES ==========================

func (s *Service) ListAntibodies(ctx context.Context, q string, includeDepleted bool) ([]Antibody, error) {
	depFilter := ""
	if !includeDepleted {
		depFilter = " AND is_depleted = FALSE"
	}
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, antibody_name, COALESCE(catalog_no,''), COALESCE(company,''),
		        COALESCE(class,''), COALESCE(antigen,''), COALESCE(host,''),
		        COALESCE(investigator,''), COALESCE(exp_id,''), COALESCE(notes,''),
		        box_id, COALESCE(location,''), COALESCE(quantity,''), is_depleted,
		        created_at, updated_at
		 FROM reagent_antibody
		 WHERE ($1 = '' OR antibody_name ILIKE '%' || $1 || '%'
		                OR catalog_no ILIKE '%' || $1 || '%'
		                OR company ILIKE '%' || $1 || '%')
		 `+depFilter+`
		 ORDER BY antibody_name`, q)
	if err != nil {
		return nil, fmt.Errorf("list antibodies: %w", err)
	}
	defer rows.Close()
	var out []Antibody
	for rows.Next() {
		var a Antibody
		if err := rows.Scan(&a.ID, &a.AntibodyName, &a.CatalogNo, &a.Company,
			&a.Class, &a.Antigen, &a.Host, &a.Investigator, &a.ExpID, &a.Notes,
			&a.BoxID, &a.Location, &a.Quantity, &a.IsDepleted,
			&a.CreatedAt, &a.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	if out == nil {
		out = []Antibody{}
	}
	return out, nil
}

func (s *Service) CreateAntibody(ctx context.Context, a Antibody, userID string) (*Antibody, error) {
	if strings.TrimSpace(a.AntibodyName) == "" {
		return nil, fmt.Errorf("%w: antibodyName is required", ErrInvalidInput)
	}
	err := s.db.QueryRowContext(ctx,
		`INSERT INTO reagent_antibody(antibody_name, catalog_no, company, class, antigen, host,
		        investigator, exp_id, notes, box_id, location, quantity, updated_by)
		 VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13)
		 RETURNING id, created_at, updated_at`,
		a.AntibodyName, a.CatalogNo, a.Company, a.Class, a.Antigen, a.Host,
		a.Investigator, a.ExpID, a.Notes, a.BoxID, a.Location, a.Quantity, userID,
	).Scan(&a.ID, &a.CreatedAt, &a.UpdatedAt)
	if err != nil {
		return nil, fmt.Errorf("create antibody: %w", err)
	}
	return &a, nil
}

func (s *Service) UpdateAntibody(ctx context.Context, id int, a Antibody, userID string) error {
	res, err := s.db.ExecContext(ctx,
		`UPDATE reagent_antibody SET antibody_name=$1, catalog_no=$2, company=$3, class=$4,
		        antigen=$5, host=$6, investigator=$7, exp_id=$8, notes=$9, box_id=$10,
		        location=$11, quantity=$12, is_depleted=$13, updated_by=$14
		 WHERE id=$15`,
		a.AntibodyName, a.CatalogNo, a.Company, a.Class, a.Antigen, a.Host,
		a.Investigator, a.ExpID, a.Notes, a.BoxID, a.Location, a.Quantity, a.IsDepleted, userID, id)
	if err != nil {
		return fmt.Errorf("update antibody: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *Service) SoftDeleteAntibody(ctx context.Context, id int, userID string) error {
	res, err := s.db.ExecContext(ctx,
		`UPDATE reagent_antibody SET is_depleted=TRUE, updated_by=$1 WHERE id=$2`, userID, id)
	if err != nil {
		return fmt.Errorf("delete antibody: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// ========================== CELL LINES ==========================

func (s *Service) ListCellLines(ctx context.Context, q string, includeDepleted bool) ([]CellLine, error) {
	depFilter := ""
	if !includeDepleted {
		depFilter = " AND is_depleted = FALSE"
	}
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, cell_line_name, COALESCE(selection,''), COALESCE(species,''),
		        COALESCE(parental_cell,''), COALESCE(medium,''), COALESCE(obtain_from,''),
		        COALESCE(cell_type,''), box_id, COALESCE(location,''), COALESCE(owner,''),
		        COALESCE(label,''), COALESCE(notes,''), is_depleted, created_at, updated_at
		 FROM reagent_cell_line
		 WHERE ($1 = '' OR cell_line_name ILIKE '%' || $1 || '%'
		                OR species ILIKE '%' || $1 || '%')
		 `+depFilter+`
		 ORDER BY cell_line_name`, q)
	if err != nil {
		return nil, fmt.Errorf("list cell lines: %w", err)
	}
	defer rows.Close()
	var out []CellLine
	for rows.Next() {
		var c CellLine
		if err := rows.Scan(&c.ID, &c.CellLineName, &c.Selection, &c.Species,
			&c.ParentalCell, &c.Medium, &c.ObtainFrom, &c.CellType, &c.BoxID,
			&c.Location, &c.Owner, &c.Label, &c.Notes, &c.IsDepleted, &c.CreatedAt, &c.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	if out == nil {
		out = []CellLine{}
	}
	return out, nil
}

func (s *Service) CreateCellLine(ctx context.Context, c CellLine, userID string) (*CellLine, error) {
	if strings.TrimSpace(c.CellLineName) == "" {
		return nil, fmt.Errorf("%w: cellLineName is required", ErrInvalidInput)
	}
	err := s.db.QueryRowContext(ctx,
		`INSERT INTO reagent_cell_line(cell_line_name, selection, species, parental_cell,
		        medium, obtain_from, cell_type, box_id, location, owner, label, notes, updated_by)
		 VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13)
		 RETURNING id, created_at, updated_at`,
		c.CellLineName, c.Selection, c.Species, c.ParentalCell,
		c.Medium, c.ObtainFrom, c.CellType, c.BoxID, c.Location, c.Owner, c.Label, c.Notes, userID,
	).Scan(&c.ID, &c.CreatedAt, &c.UpdatedAt)
	if err != nil {
		return nil, fmt.Errorf("create cell line: %w", err)
	}
	return &c, nil
}

func (s *Service) UpdateCellLine(ctx context.Context, id int, c CellLine, userID string) error {
	res, err := s.db.ExecContext(ctx,
		`UPDATE reagent_cell_line SET cell_line_name=$1, selection=$2, species=$3,
		        parental_cell=$4, medium=$5, obtain_from=$6, cell_type=$7, box_id=$8,
		        location=$9, owner=$10, label=$11, notes=$12, is_depleted=$13, updated_by=$14
		 WHERE id=$15`,
		c.CellLineName, c.Selection, c.Species, c.ParentalCell,
		c.Medium, c.ObtainFrom, c.CellType, c.BoxID, c.Location, c.Owner, c.Label, c.Notes, c.IsDepleted, userID, id)
	if err != nil {
		return fmt.Errorf("update cell line: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *Service) SoftDeleteCellLine(ctx context.Context, id int, userID string) error {
	res, err := s.db.ExecContext(ctx,
		`UPDATE reagent_cell_line SET is_depleted=TRUE, updated_by=$1 WHERE id=$2`, userID, id)
	if err != nil {
		return fmt.Errorf("delete cell line: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// ========================== VIRUSES ==========================

func (s *Service) ListViruses(ctx context.Context, q string, includeDepleted bool) ([]Virus, error) {
	depFilter := ""
	if !includeDepleted {
		depFilter = " AND is_depleted = FALSE"
	}
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, virus_name, COALESCE(virus_type,''), box_id, COALESCE(location,''),
		        COALESCE(owner,''), COALESCE(label,''), COALESCE(notes,''), is_depleted,
		        created_at, updated_at
		 FROM reagent_virus
		 WHERE ($1 = '' OR virus_name ILIKE '%' || $1 || '%')
		 `+depFilter+`
		 ORDER BY virus_name`, q)
	if err != nil {
		return nil, fmt.Errorf("list viruses: %w", err)
	}
	defer rows.Close()
	var out []Virus
	for rows.Next() {
		var v Virus
		if err := rows.Scan(&v.ID, &v.VirusName, &v.VirusType, &v.BoxID, &v.Location,
			&v.Owner, &v.Label, &v.Notes, &v.IsDepleted, &v.CreatedAt, &v.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	if out == nil {
		out = []Virus{}
	}
	return out, nil
}

func (s *Service) CreateVirus(ctx context.Context, v Virus, userID string) (*Virus, error) {
	if strings.TrimSpace(v.VirusName) == "" {
		return nil, fmt.Errorf("%w: virusName is required", ErrInvalidInput)
	}
	err := s.db.QueryRowContext(ctx,
		`INSERT INTO reagent_virus(virus_name, virus_type, box_id, location, owner, label, notes, updated_by)
		 VALUES($1,$2,$3,$4,$5,$6,$7,$8) RETURNING id, created_at, updated_at`,
		v.VirusName, v.VirusType, v.BoxID, v.Location, v.Owner, v.Label, v.Notes, userID,
	).Scan(&v.ID, &v.CreatedAt, &v.UpdatedAt)
	if err != nil {
		return nil, fmt.Errorf("create virus: %w", err)
	}
	return &v, nil
}

func (s *Service) UpdateVirus(ctx context.Context, id int, v Virus, userID string) error {
	res, err := s.db.ExecContext(ctx,
		`UPDATE reagent_virus SET virus_name=$1, virus_type=$2, box_id=$3, location=$4,
		        owner=$5, label=$6, notes=$7, is_depleted=$8, updated_by=$9 WHERE id=$10`,
		v.VirusName, v.VirusType, v.BoxID, v.Location, v.Owner, v.Label, v.Notes, v.IsDepleted, userID, id)
	if err != nil {
		return fmt.Errorf("update virus: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *Service) SoftDeleteVirus(ctx context.Context, id int, userID string) error {
	res, err := s.db.ExecContext(ctx,
		`UPDATE reagent_virus SET is_depleted=TRUE, updated_by=$1 WHERE id=$2`, userID, id)
	if err != nil {
		return fmt.Errorf("delete virus: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// ========================== DNA ==========================

func (s *Service) ListDNA(ctx context.Context, q string, includeDepleted bool) ([]DNA, error) {
	depFilter := ""
	if !includeDepleted {
		depFilter = " AND is_depleted = FALSE"
	}
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, dna_name, COALESCE(dna_type,''), box_id, COALESCE(location,''),
		        COALESCE(owner,''), COALESCE(label,''), COALESCE(notes,''), is_depleted,
		        created_at, updated_at
		 FROM reagent_dna
		 WHERE ($1 = '' OR dna_name ILIKE '%' || $1 || '%')
		 `+depFilter+`
		 ORDER BY dna_name`, q)
	if err != nil {
		return nil, fmt.Errorf("list dna: %w", err)
	}
	defer rows.Close()
	var out []DNA
	for rows.Next() {
		var d DNA
		if err := rows.Scan(&d.ID, &d.DNAName, &d.DNAType, &d.BoxID, &d.Location,
			&d.Owner, &d.Label, &d.Notes, &d.IsDepleted, &d.CreatedAt, &d.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, d)
	}
	if out == nil {
		out = []DNA{}
	}
	return out, nil
}

func (s *Service) CreateDNA(ctx context.Context, d DNA, userID string) (*DNA, error) {
	if strings.TrimSpace(d.DNAName) == "" {
		return nil, fmt.Errorf("%w: dnaName is required", ErrInvalidInput)
	}
	err := s.db.QueryRowContext(ctx,
		`INSERT INTO reagent_dna(dna_name, dna_type, box_id, location, owner, label, notes, updated_by)
		 VALUES($1,$2,$3,$4,$5,$6,$7,$8) RETURNING id, created_at, updated_at`,
		d.DNAName, d.DNAType, d.BoxID, d.Location, d.Owner, d.Label, d.Notes, userID,
	).Scan(&d.ID, &d.CreatedAt, &d.UpdatedAt)
	if err != nil {
		return nil, fmt.Errorf("create dna: %w", err)
	}
	return &d, nil
}

func (s *Service) UpdateDNA(ctx context.Context, id int, d DNA, userID string) error {
	res, err := s.db.ExecContext(ctx,
		`UPDATE reagent_dna SET dna_name=$1, dna_type=$2, box_id=$3, location=$4,
		        owner=$5, label=$6, notes=$7, is_depleted=$8, updated_by=$9 WHERE id=$10`,
		d.DNAName, d.DNAType, d.BoxID, d.Location, d.Owner, d.Label, d.Notes, d.IsDepleted, userID, id)
	if err != nil {
		return fmt.Errorf("update dna: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *Service) SoftDeleteDNA(ctx context.Context, id int, userID string) error {
	res, err := s.db.ExecContext(ctx,
		`UPDATE reagent_dna SET is_depleted=TRUE, updated_by=$1 WHERE id=$2`, userID, id)
	if err != nil {
		return fmt.Errorf("delete dna: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// ========================== OLIGOS ==========================

func (s *Service) ListOligos(ctx context.Context, q string, includeDepleted bool) ([]Oligo, error) {
	depFilter := ""
	if !includeDepleted {
		depFilter = " AND is_depleted = FALSE"
	}
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, oligo_name, COALESCE(sequence,''), COALESCE(oligo_type,''),
		        box_id, COALESCE(location,''), COALESCE(owner,''), COALESCE(label,''),
		        COALESCE(notes,''), is_depleted, created_at, updated_at
		 FROM reagent_oligo
		 WHERE ($1 = '' OR oligo_name ILIKE '%' || $1 || '%'
		                OR sequence ILIKE '%' || $1 || '%')
		 `+depFilter+`
		 ORDER BY oligo_name`, q)
	if err != nil {
		return nil, fmt.Errorf("list oligos: %w", err)
	}
	defer rows.Close()
	var out []Oligo
	for rows.Next() {
		var o Oligo
		if err := rows.Scan(&o.ID, &o.OligoName, &o.Sequence, &o.OligoType, &o.BoxID,
			&o.Location, &o.Owner, &o.Label, &o.Notes, &o.IsDepleted,
			&o.CreatedAt, &o.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, o)
	}
	if out == nil {
		out = []Oligo{}
	}
	return out, nil
}

func (s *Service) CreateOligo(ctx context.Context, o Oligo, userID string) (*Oligo, error) {
	if strings.TrimSpace(o.OligoName) == "" {
		return nil, fmt.Errorf("%w: oligoName is required", ErrInvalidInput)
	}
	err := s.db.QueryRowContext(ctx,
		`INSERT INTO reagent_oligo(oligo_name, sequence, oligo_type, box_id, location, owner, label, notes, updated_by)
		 VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9) RETURNING id, created_at, updated_at`,
		o.OligoName, o.Sequence, o.OligoType, o.BoxID, o.Location, o.Owner, o.Label, o.Notes, userID,
	).Scan(&o.ID, &o.CreatedAt, &o.UpdatedAt)
	if err != nil {
		return nil, fmt.Errorf("create oligo: %w", err)
	}
	return &o, nil
}

func (s *Service) UpdateOligo(ctx context.Context, id int, o Oligo, userID string) error {
	res, err := s.db.ExecContext(ctx,
		`UPDATE reagent_oligo SET oligo_name=$1, sequence=$2, oligo_type=$3, box_id=$4,
		        location=$5, owner=$6, label=$7, notes=$8, is_depleted=$9, updated_by=$10
		 WHERE id=$11`,
		o.OligoName, o.Sequence, o.OligoType, o.BoxID, o.Location, o.Owner, o.Label, o.Notes, o.IsDepleted, userID, id)
	if err != nil {
		return fmt.Errorf("update oligo: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *Service) SoftDeleteOligo(ctx context.Context, id int, userID string) error {
	res, err := s.db.ExecContext(ctx,
		`UPDATE reagent_oligo SET is_depleted=TRUE, updated_by=$1 WHERE id=$2`, userID, id)
	if err != nil {
		return fmt.Errorf("delete oligo: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// ========================== CHEMICALS ==========================

func (s *Service) ListChemicals(ctx context.Context, q string, includeDepleted bool) ([]Chemical, error) {
	depFilter := ""
	if !includeDepleted {
		depFilter = " AND is_depleted = FALSE"
	}
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, chemical_name, COALESCE(catalog_no,''), COALESCE(company,''),
		        COALESCE(chem_type,''), box_id, COALESCE(location,''), COALESCE(owner,''),
		        COALESCE(label,''), COALESCE(notes,''), is_depleted, created_at, updated_at
		 FROM reagent_chemical
		 WHERE ($1 = '' OR chemical_name ILIKE '%' || $1 || '%'
		                OR catalog_no ILIKE '%' || $1 || '%'
		                OR company ILIKE '%' || $1 || '%')
		 `+depFilter+`
		 ORDER BY chemical_name`, q)
	if err != nil {
		return nil, fmt.Errorf("list chemicals: %w", err)
	}
	defer rows.Close()
	var out []Chemical
	for rows.Next() {
		var c Chemical
		if err := rows.Scan(&c.ID, &c.ChemicalName, &c.CatalogNo, &c.Company, &c.ChemType,
			&c.BoxID, &c.Location, &c.Owner, &c.Label, &c.Notes, &c.IsDepleted,
			&c.CreatedAt, &c.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	if out == nil {
		out = []Chemical{}
	}
	return out, nil
}

func (s *Service) CreateChemical(ctx context.Context, c Chemical, userID string) (*Chemical, error) {
	if strings.TrimSpace(c.ChemicalName) == "" {
		return nil, fmt.Errorf("%w: chemicalName is required", ErrInvalidInput)
	}
	err := s.db.QueryRowContext(ctx,
		`INSERT INTO reagent_chemical(chemical_name, catalog_no, company, chem_type, box_id,
		        location, owner, label, notes, updated_by)
		 VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)
		 RETURNING id, created_at, updated_at`,
		c.ChemicalName, c.CatalogNo, c.Company, c.ChemType, c.BoxID,
		c.Location, c.Owner, c.Label, c.Notes, userID,
	).Scan(&c.ID, &c.CreatedAt, &c.UpdatedAt)
	if err != nil {
		return nil, fmt.Errorf("create chemical: %w", err)
	}
	return &c, nil
}

func (s *Service) UpdateChemical(ctx context.Context, id int, c Chemical, userID string) error {
	res, err := s.db.ExecContext(ctx,
		`UPDATE reagent_chemical SET chemical_name=$1, catalog_no=$2, company=$3, chem_type=$4,
		        box_id=$5, location=$6, owner=$7, label=$8, notes=$9, is_depleted=$10, updated_by=$11
		 WHERE id=$12`,
		c.ChemicalName, c.CatalogNo, c.Company, c.ChemType, c.BoxID,
		c.Location, c.Owner, c.Label, c.Notes, c.IsDepleted, userID, id)
	if err != nil {
		return fmt.Errorf("update chemical: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *Service) SoftDeleteChemical(ctx context.Context, id int, userID string) error {
	res, err := s.db.ExecContext(ctx,
		`UPDATE reagent_chemical SET is_depleted=TRUE, updated_by=$1 WHERE id=$2`, userID, id)
	if err != nil {
		return fmt.Errorf("delete chemical: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// ========================== MOLECULAR ==========================

func (s *Service) ListMolecular(ctx context.Context, q string, includeDepleted bool) ([]Molecular, error) {
	depFilter := ""
	if !includeDepleted {
		depFilter = " AND is_depleted = FALSE"
	}
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, mr_name, COALESCE(mr_type,''), box_id, COALESCE(location,''),
		        COALESCE(position,''), COALESCE(owner,''), COALESCE(label,''),
		        COALESCE(notes,''), is_depleted, created_at, updated_at
		 FROM reagent_molecular
		 WHERE ($1 = '' OR mr_name ILIKE '%' || $1 || '%')
		 `+depFilter+`
		 ORDER BY mr_name`, q)
	if err != nil {
		return nil, fmt.Errorf("list molecular: %w", err)
	}
	defer rows.Close()
	var out []Molecular
	for rows.Next() {
		var m Molecular
		if err := rows.Scan(&m.ID, &m.MRName, &m.MRType, &m.BoxID, &m.Location,
			&m.Position, &m.Owner, &m.Label, &m.Notes, &m.IsDepleted,
			&m.CreatedAt, &m.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	if out == nil {
		out = []Molecular{}
	}
	return out, nil
}

func (s *Service) CreateMolecular(ctx context.Context, m Molecular, userID string) (*Molecular, error) {
	if strings.TrimSpace(m.MRName) == "" {
		return nil, fmt.Errorf("%w: mrName is required", ErrInvalidInput)
	}
	err := s.db.QueryRowContext(ctx,
		`INSERT INTO reagent_molecular(mr_name, mr_type, box_id, location, position, owner, label, notes, updated_by)
		 VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9) RETURNING id, created_at, updated_at`,
		m.MRName, m.MRType, m.BoxID, m.Location, m.Position, m.Owner, m.Label, m.Notes, userID,
	).Scan(&m.ID, &m.CreatedAt, &m.UpdatedAt)
	if err != nil {
		return nil, fmt.Errorf("create molecular: %w", err)
	}
	return &m, nil
}

func (s *Service) UpdateMolecular(ctx context.Context, id int, m Molecular, userID string) error {
	res, err := s.db.ExecContext(ctx,
		`UPDATE reagent_molecular SET mr_name=$1, mr_type=$2, box_id=$3, location=$4,
		        position=$5, owner=$6, label=$7, notes=$8, is_depleted=$9, updated_by=$10
		 WHERE id=$11`,
		m.MRName, m.MRType, m.BoxID, m.Location, m.Position, m.Owner, m.Label, m.Notes, m.IsDepleted, userID, id)
	if err != nil {
		return fmt.Errorf("update molecular: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *Service) SoftDeleteMolecular(ctx context.Context, id int, userID string) error {
	res, err := s.db.ExecContext(ctx,
		`UPDATE reagent_molecular SET is_depleted=TRUE, updated_by=$1 WHERE id=$2`, userID, id)
	if err != nil {
		return fmt.Errorf("delete molecular: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// ========================== CROSS-TYPE SEARCH ==========================

func (s *Service) SearchAll(ctx context.Context, q string) ([]SearchResult, error) {
	if strings.TrimSpace(q) == "" {
		return []SearchResult{}, nil
	}

	var results []SearchResult

	// Helper to run a query and append results
	type queryDef struct {
		typ   string
		query string
	}
	queries := []queryDef{
		{"antibody", `SELECT id, antibody_name FROM reagent_antibody WHERE antibody_name ILIKE '%' || $1 || '%' OR catalog_no ILIKE '%' || $1 || '%' ORDER BY antibody_name LIMIT 20`},
		{"cellLine", `SELECT id, cell_line_name FROM reagent_cell_line WHERE cell_line_name ILIKE '%' || $1 || '%' ORDER BY cell_line_name LIMIT 20`},
		{"virus", `SELECT id, virus_name FROM reagent_virus WHERE virus_name ILIKE '%' || $1 || '%' ORDER BY virus_name LIMIT 20`},
		{"dna", `SELECT id, dna_name FROM reagent_dna WHERE dna_name ILIKE '%' || $1 || '%' ORDER BY dna_name LIMIT 20`},
		{"oligo", `SELECT id, oligo_name FROM reagent_oligo WHERE oligo_name ILIKE '%' || $1 || '%' OR sequence ILIKE '%' || $1 || '%' ORDER BY oligo_name LIMIT 20`},
		{"chemical", `SELECT id, chemical_name FROM reagent_chemical WHERE chemical_name ILIKE '%' || $1 || '%' OR catalog_no ILIKE '%' || $1 || '%' ORDER BY chemical_name LIMIT 20`},
		{"molecular", `SELECT id, mr_name FROM reagent_molecular WHERE mr_name ILIKE '%' || $1 || '%' ORDER BY mr_name LIMIT 20`},
	}

	for _, qd := range queries {
		rows, err := s.db.QueryContext(ctx, qd.query, q)
		if err != nil {
			continue
		}
		for rows.Next() {
			var id int
			var name string
			if err := rows.Scan(&id, &name); err != nil {
				continue
			}
			results = append(results, SearchResult{Type: qd.typ, ID: id, Name: name})
		}
		rows.Close()
	}

	if results == nil {
		results = []SearchResult{}
	}
	return results, nil
}

// ========================== IMPORT FROM CSV JSON ==========================

// ImportBulk allows bulk import via JSON array for any reagent type.
// Used by the CSV import tool and could be used for REST bulk import.
type BulkImportResult struct {
	Imported int      `json:"imported"`
	Errors   []string `json:"errors,omitempty"`
}

func (s *Service) BulkImportAntibodies(ctx context.Context, items []Antibody, userID string) (*BulkImportResult, error) {
	result := &BulkImportResult{}
	for _, a := range items {
		_, err := s.CreateAntibody(ctx, a, userID)
		if err != nil {
			result.Errors = append(result.Errors, fmt.Sprintf("row %q: %v", a.AntibodyName, err))
			continue
		}
		result.Imported++
	}
	return result, nil
}

// Helper to unmarshal generic JSON for bulk import endpoints
func UnmarshalJSON(data []byte, v interface{}) error {
	return json.Unmarshal(data, v)
}
