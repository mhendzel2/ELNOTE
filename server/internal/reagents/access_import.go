package reagents

import (
	"bytes"
	"context"
	"encoding/csv"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

type AccessImportInput struct {
	Filename  string
	FileBytes []byte
	UserID    string
}

type AccessImportResourceResult struct {
	Resource string   `json:"resource"`
	Table    string   `json:"table,omitempty"`
	Rows     int      `json:"rows"`
	Imported int      `json:"imported"`
	Errors   []string `json:"errors,omitempty"`
	Skipped  bool     `json:"skipped"`
	Message  string   `json:"message,omitempty"`
}

type AccessImportResult struct {
	FileName         string                       `json:"fileName"`
	TotalImported    int                          `json:"totalImported"`
	Tables           []string                     `json:"tables"`
	Results          []AccessImportResourceResult `json:"results"`
	UnmappedTables   []string                     `json:"unmappedTables,omitempty"`
	MissingResources []string                     `json:"missingResources,omitempty"`
}

type accessSpec struct {
	Resource        string
	Keywords        []string
	RequiredKeys    []string
	OptionalKeys    []string
	Aliases         map[string]string
	LegacyIDAliases []string
}

type accessTable struct {
	Name    string
	Headers []string
	Rows    [][]string
}

type accessPreparedRow struct {
	RowNumber int
	Values    map[string]any
	LegacyID  *int
}

var accessImportSpecs = []accessSpec{
	{
		Resource:        "storage",
		Keywords:        []string{"storage"},
		RequiredKeys:    []string{"name"},
		OptionalKeys:    []string{"locationType", "description"},
		LegacyIDAliases: []string{"id", "storage_id", "storageid"},
		Aliases: map[string]string{
			"type":          "locationType",
			"location_type": "locationType",
		},
	},
	{
		Resource:        "boxes",
		Keywords:        []string{"box", "boxes"},
		RequiredKeys:    []string{"boxNo"},
		OptionalKeys:    []string{"boxType", "owner", "label", "location", "drawer", "position", "storageId"},
		LegacyIDAliases: []string{"id", "box_id", "boxid"},
		Aliases: map[string]string{
			"box_number": "boxNo",
			"storage_id": "storageId",
		},
	},
	{
		Resource:        "antibodies",
		Keywords:        []string{"antibody", "antibodies"},
		RequiredKeys:    []string{"antibodyName"},
		OptionalKeys:    []string{"catalogNo", "company", "lotNumber", "expiryDate", "class", "antigen", "host", "investigator", "expId", "notes", "boxId", "location", "quantity", "isDepleted"},
		LegacyIDAliases: []string{"id", "antibody_id", "antibodyid"},
		Aliases: map[string]string{
			"catalog_number":  "catalogNo",
			"expiry":          "expiryDate",
			"exp_date":        "expiryDate",
			"expiration_date": "expiryDate",
			"class_name":      "class",
			"box_id":          "boxId",
		},
	},
	{
		Resource:        "cell-lines",
		Keywords:        []string{"cellline", "celllines", "cell_line"},
		RequiredKeys:    []string{"cellLineName"},
		OptionalKeys:    []string{"lotNumber", "expiryDate", "selection", "species", "parentalCell", "medium", "obtainFrom", "cellType", "boxId", "location", "owner", "label", "notes", "isDepleted"},
		LegacyIDAliases: []string{"id", "cell_line_id", "celllineid"},
		Aliases: map[string]string{
			"cell_line_name":  "cellLineName",
			"expiry":          "expiryDate",
			"exp_date":        "expiryDate",
			"expiration_date": "expiryDate",
			"obtain_from":     "obtainFrom",
			"cell_type":       "cellType",
			"box_id":          "boxId",
		},
	},
	{
		Resource:        "viruses",
		Keywords:        []string{"virus", "viruses"},
		RequiredKeys:    []string{"virusName"},
		OptionalKeys:    []string{"virusType", "lotNumber", "expiryDate", "boxId", "location", "owner", "label", "notes", "isDepleted"},
		LegacyIDAliases: []string{"id", "virus_id", "virusid"},
		Aliases: map[string]string{
			"virus_type":      "virusType",
			"expiry":          "expiryDate",
			"exp_date":        "expiryDate",
			"expiration_date": "expiryDate",
			"box_id":          "boxId",
		},
	},
	{
		Resource:        "dna",
		Keywords:        []string{"dna", "construct"},
		RequiredKeys:    []string{"dnaName"},
		OptionalKeys:    []string{"dnaType", "lotNumber", "expiryDate", "boxId", "location", "owner", "label", "notes", "isDepleted"},
		LegacyIDAliases: []string{"id", "dna_id", "dnaid"},
		Aliases: map[string]string{
			"dna_type":        "dnaType",
			"expiry":          "expiryDate",
			"exp_date":        "expiryDate",
			"expiration_date": "expiryDate",
			"box_id":          "boxId",
		},
	},
	{
		Resource:        "oligos",
		Keywords:        []string{"oligo", "oligos", "primer"},
		RequiredKeys:    []string{"oligoName"},
		OptionalKeys:    []string{"sequence", "oligoType", "lotNumber", "expiryDate", "boxId", "location", "owner", "label", "notes", "isDepleted"},
		LegacyIDAliases: []string{"id", "oligo_id", "oligoid"},
		Aliases: map[string]string{
			"oligo_type":      "oligoType",
			"expiry":          "expiryDate",
			"exp_date":        "expiryDate",
			"expiration_date": "expiryDate",
			"box_id":          "boxId",
		},
	},
	{
		Resource:        "chemicals",
		Keywords:        []string{"chemical", "chemicals"},
		RequiredKeys:    []string{"chemicalName"},
		OptionalKeys:    []string{"catalogNo", "company", "chemType", "lotNumber", "expiryDate", "boxId", "location", "owner", "label", "notes", "isDepleted"},
		LegacyIDAliases: []string{"id", "chemical_id", "chemicalid"},
		Aliases: map[string]string{
			"catalog_number":  "catalogNo",
			"chem_type":       "chemType",
			"expiry":          "expiryDate",
			"exp_date":        "expiryDate",
			"expiration_date": "expiryDate",
			"box_id":          "boxId",
		},
	},
	{
		Resource:        "molecular",
		Keywords:        []string{"molecular", "mitem", "mitems", "mr"},
		RequiredKeys:    []string{"mrName"},
		OptionalKeys:    []string{"mrType", "lotNumber", "expiryDate", "boxId", "location", "position", "owner", "label", "notes", "isDepleted"},
		LegacyIDAliases: []string{"id", "molecular_id", "mr_id", "mrid"},
		Aliases: map[string]string{
			"mr_type":         "mrType",
			"expiry":          "expiryDate",
			"exp_date":        "expiryDate",
			"expiration_date": "expiryDate",
			"box_id":          "boxId",
		},
	},
}

func (s *Service) ImportAccessDatabase(ctx context.Context, in AccessImportInput) (*AccessImportResult, error) {
	if len(in.FileBytes) == 0 {
		return nil, fmt.Errorf("%w: file is required", ErrInvalidInput)
	}
	if strings.TrimSpace(in.UserID) == "" {
		return nil, fmt.Errorf("%w: user is required", ErrInvalidInput)
	}
	ext := strings.ToLower(strings.TrimSpace(filepath.Ext(in.Filename)))
	if ext != ".mdb" && ext != ".accdb" {
		return nil, fmt.Errorf("%w: file must end with .mdb or .accdb", ErrInvalidInput)
	}
	if err := ensureMDBTools(); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalidInput, err)
	}

	tmp, err := os.CreateTemp("", "elnote-access-import-*"+ext)
	if err != nil {
		return nil, fmt.Errorf("create temp file: %w", err)
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath)
	if _, err := tmp.Write(in.FileBytes); err != nil {
		tmp.Close()
		return nil, fmt.Errorf("write temp file: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return nil, fmt.Errorf("close temp file: %w", err)
	}

	tables, err := listAccessTables(ctx, tmpPath)
	if err != nil {
		return nil, err
	}
	if len(tables) == 0 {
		return nil, fmt.Errorf("%w: no tables found in access file", ErrInvalidInput)
	}

	var exports []accessTable
	for _, tableName := range tables {
		exported, err := exportAccessTable(ctx, tmpPath, tableName)
		if err != nil {
			continue
		}
		exports = append(exports, exported)
	}
	if len(exports) == 0 {
		return nil, fmt.Errorf("%w: no readable tables found", ErrInvalidInput)
	}

	selected := matchAccessTables(exports)
	tableSet := make(map[string]struct{}, len(exports))
	for _, t := range exports {
		tableSet[t.Name] = struct{}{}
	}
	for _, t := range selected {
		delete(tableSet, t.Name)
	}
	unmappedTables := make([]string, 0, len(tableSet))
	for table := range tableSet {
		unmappedTables = append(unmappedTables, table)
	}
	sortStrings(unmappedTables)

	storageIDMap := map[int]int{}
	boxIDMap := map[int]int{}

	var results []AccessImportResourceResult
	totalImported := 0
	var missingResources []string

	for _, spec := range accessImportSpecs {
		table, ok := selected[spec.Resource]
		if !ok {
			results = append(results, AccessImportResourceResult{
				Resource: spec.Resource,
				Skipped:  true,
				Message:  "no matching table found",
			})
			missingResources = append(missingResources, spec.Resource)
			continue
		}

		rows, prepErrors := prepareRows(spec, table)
		entry := AccessImportResourceResult{
			Resource: spec.Resource,
			Table:    table.Name,
			Rows:     len(rows),
			Errors:   prepErrors,
		}

		for _, row := range rows {
			switch spec.Resource {
			case "storage":
				item := Storage{
					Name:         valueString(row.Values, "name"),
					LocationType: valueString(row.Values, "locationType"),
					Description:  valueString(row.Values, "description"),
				}
				created, err := s.CreateStorage(ctx, item, in.UserID)
				if err != nil {
					entry.Errors = append(entry.Errors, fmt.Sprintf("row %d: %v", row.RowNumber, err))
					continue
				}
				entry.Imported++
				if row.LegacyID != nil {
					storageIDMap[*row.LegacyID] = created.ID
				}
			case "boxes":
				var mappedStorageID *int
				if legacyStorageID, ok := valueInt(row.Values, "storageId"); ok {
					mapped, found := storageIDMap[legacyStorageID]
					if !found {
						entry.Errors = append(entry.Errors, fmt.Sprintf("row %d: storageId %d not found in imported storage records", row.RowNumber, legacyStorageID))
						continue
					}
					mappedStorageID = &mapped
				}
				item := Box{
					BoxNo:     valueString(row.Values, "boxNo"),
					BoxType:   valueString(row.Values, "boxType"),
					Owner:     valueString(row.Values, "owner"),
					Label:     valueString(row.Values, "label"),
					Location:  valueString(row.Values, "location"),
					Drawer:    valueString(row.Values, "drawer"),
					Position:  valueString(row.Values, "position"),
					StorageID: mappedStorageID,
				}
				created, err := s.CreateBox(ctx, item, in.UserID)
				if err != nil {
					entry.Errors = append(entry.Errors, fmt.Sprintf("row %d: %v", row.RowNumber, err))
					continue
				}
				entry.Imported++
				if row.LegacyID != nil {
					boxIDMap[*row.LegacyID] = created.ID
				}
			case "antibodies":
				boxID, ok := mapLegacyBoxID(row, boxIDMap)
				if !ok {
					entry.Errors = append(entry.Errors, fmt.Sprintf("row %d: boxId reference not found", row.RowNumber))
					continue
				}
				item := Antibody{
					AntibodyName: valueString(row.Values, "antibodyName"),
					CatalogNo:    valueString(row.Values, "catalogNo"),
					Company:      valueString(row.Values, "company"),
					LotNumber:    valueString(row.Values, "lotNumber"),
					ExpiryDate:   valueString(row.Values, "expiryDate"),
					Class:        valueString(row.Values, "class"),
					Antigen:      valueString(row.Values, "antigen"),
					Host:         valueString(row.Values, "host"),
					Investigator: valueString(row.Values, "investigator"),
					ExpID:        valueString(row.Values, "expId"),
					Notes:        valueString(row.Values, "notes"),
					BoxID:        boxID,
					Location:     valueString(row.Values, "location"),
					Quantity:     valueString(row.Values, "quantity"),
				}
				created, err := s.CreateAntibody(ctx, item, in.UserID)
				if err != nil {
					entry.Errors = append(entry.Errors, fmt.Sprintf("row %d: %v", row.RowNumber, err))
					continue
				}
				entry.Imported++
				if valueBool(row.Values, "isDepleted") {
					_ = s.SoftDeleteAntibody(ctx, created.ID, in.UserID)
				}
			case "cell-lines":
				boxID, ok := mapLegacyBoxID(row, boxIDMap)
				if !ok {
					entry.Errors = append(entry.Errors, fmt.Sprintf("row %d: boxId reference not found", row.RowNumber))
					continue
				}
				item := CellLine{
					CellLineName: valueString(row.Values, "cellLineName"),
					LotNumber:    valueString(row.Values, "lotNumber"),
					ExpiryDate:   valueString(row.Values, "expiryDate"),
					Selection:    valueString(row.Values, "selection"),
					Species:      valueString(row.Values, "species"),
					ParentalCell: valueString(row.Values, "parentalCell"),
					Medium:       valueString(row.Values, "medium"),
					ObtainFrom:   valueString(row.Values, "obtainFrom"),
					CellType:     valueString(row.Values, "cellType"),
					BoxID:        boxID,
					Location:     valueString(row.Values, "location"),
					Owner:        valueString(row.Values, "owner"),
					Label:        valueString(row.Values, "label"),
					Notes:        valueString(row.Values, "notes"),
				}
				created, err := s.CreateCellLine(ctx, item, in.UserID)
				if err != nil {
					entry.Errors = append(entry.Errors, fmt.Sprintf("row %d: %v", row.RowNumber, err))
					continue
				}
				entry.Imported++
				if valueBool(row.Values, "isDepleted") {
					_ = s.SoftDeleteCellLine(ctx, created.ID, in.UserID)
				}
			case "viruses":
				boxID, ok := mapLegacyBoxID(row, boxIDMap)
				if !ok {
					entry.Errors = append(entry.Errors, fmt.Sprintf("row %d: boxId reference not found", row.RowNumber))
					continue
				}
				item := Virus{
					VirusName:  valueString(row.Values, "virusName"),
					VirusType:  valueString(row.Values, "virusType"),
					LotNumber:  valueString(row.Values, "lotNumber"),
					ExpiryDate: valueString(row.Values, "expiryDate"),
					BoxID:      boxID,
					Location:   valueString(row.Values, "location"),
					Owner:      valueString(row.Values, "owner"),
					Label:      valueString(row.Values, "label"),
					Notes:      valueString(row.Values, "notes"),
				}
				created, err := s.CreateVirus(ctx, item, in.UserID)
				if err != nil {
					entry.Errors = append(entry.Errors, fmt.Sprintf("row %d: %v", row.RowNumber, err))
					continue
				}
				entry.Imported++
				if valueBool(row.Values, "isDepleted") {
					_ = s.SoftDeleteVirus(ctx, created.ID, in.UserID)
				}
			case "dna":
				boxID, ok := mapLegacyBoxID(row, boxIDMap)
				if !ok {
					entry.Errors = append(entry.Errors, fmt.Sprintf("row %d: boxId reference not found", row.RowNumber))
					continue
				}
				item := DNA{
					DNAName:    valueString(row.Values, "dnaName"),
					DNAType:    valueString(row.Values, "dnaType"),
					LotNumber:  valueString(row.Values, "lotNumber"),
					ExpiryDate: valueString(row.Values, "expiryDate"),
					BoxID:      boxID,
					Location:   valueString(row.Values, "location"),
					Owner:      valueString(row.Values, "owner"),
					Label:      valueString(row.Values, "label"),
					Notes:      valueString(row.Values, "notes"),
				}
				created, err := s.CreateDNA(ctx, item, in.UserID)
				if err != nil {
					entry.Errors = append(entry.Errors, fmt.Sprintf("row %d: %v", row.RowNumber, err))
					continue
				}
				entry.Imported++
				if valueBool(row.Values, "isDepleted") {
					_ = s.SoftDeleteDNA(ctx, created.ID, in.UserID)
				}
			case "oligos":
				boxID, ok := mapLegacyBoxID(row, boxIDMap)
				if !ok {
					entry.Errors = append(entry.Errors, fmt.Sprintf("row %d: boxId reference not found", row.RowNumber))
					continue
				}
				item := Oligo{
					OligoName:  valueString(row.Values, "oligoName"),
					Sequence:   valueString(row.Values, "sequence"),
					OligoType:  valueString(row.Values, "oligoType"),
					LotNumber:  valueString(row.Values, "lotNumber"),
					ExpiryDate: valueString(row.Values, "expiryDate"),
					BoxID:      boxID,
					Location:   valueString(row.Values, "location"),
					Owner:      valueString(row.Values, "owner"),
					Label:      valueString(row.Values, "label"),
					Notes:      valueString(row.Values, "notes"),
				}
				created, err := s.CreateOligo(ctx, item, in.UserID)
				if err != nil {
					entry.Errors = append(entry.Errors, fmt.Sprintf("row %d: %v", row.RowNumber, err))
					continue
				}
				entry.Imported++
				if valueBool(row.Values, "isDepleted") {
					_ = s.SoftDeleteOligo(ctx, created.ID, in.UserID)
				}
			case "chemicals":
				boxID, ok := mapLegacyBoxID(row, boxIDMap)
				if !ok {
					entry.Errors = append(entry.Errors, fmt.Sprintf("row %d: boxId reference not found", row.RowNumber))
					continue
				}
				item := Chemical{
					ChemicalName: valueString(row.Values, "chemicalName"),
					CatalogNo:    valueString(row.Values, "catalogNo"),
					Company:      valueString(row.Values, "company"),
					ChemType:     valueString(row.Values, "chemType"),
					LotNumber:    valueString(row.Values, "lotNumber"),
					ExpiryDate:   valueString(row.Values, "expiryDate"),
					BoxID:        boxID,
					Location:     valueString(row.Values, "location"),
					Owner:        valueString(row.Values, "owner"),
					Label:        valueString(row.Values, "label"),
					Notes:        valueString(row.Values, "notes"),
				}
				created, err := s.CreateChemical(ctx, item, in.UserID)
				if err != nil {
					entry.Errors = append(entry.Errors, fmt.Sprintf("row %d: %v", row.RowNumber, err))
					continue
				}
				entry.Imported++
				if valueBool(row.Values, "isDepleted") {
					_ = s.SoftDeleteChemical(ctx, created.ID, in.UserID)
				}
			case "molecular":
				boxID, ok := mapLegacyBoxID(row, boxIDMap)
				if !ok {
					entry.Errors = append(entry.Errors, fmt.Sprintf("row %d: boxId reference not found", row.RowNumber))
					continue
				}
				item := Molecular{
					MRName:     valueString(row.Values, "mrName"),
					MRType:     valueString(row.Values, "mrType"),
					LotNumber:  valueString(row.Values, "lotNumber"),
					ExpiryDate: valueString(row.Values, "expiryDate"),
					BoxID:      boxID,
					Location:   valueString(row.Values, "location"),
					Position:   valueString(row.Values, "position"),
					Owner:      valueString(row.Values, "owner"),
					Label:      valueString(row.Values, "label"),
					Notes:      valueString(row.Values, "notes"),
				}
				created, err := s.CreateMolecular(ctx, item, in.UserID)
				if err != nil {
					entry.Errors = append(entry.Errors, fmt.Sprintf("row %d: %v", row.RowNumber, err))
					continue
				}
				entry.Imported++
				if valueBool(row.Values, "isDepleted") {
					_ = s.SoftDeleteMolecular(ctx, created.ID, in.UserID)
				}
			}
		}

		totalImported += entry.Imported
		results = append(results, entry)
	}

	return &AccessImportResult{
		FileName:         in.Filename,
		TotalImported:    totalImported,
		Tables:           tables,
		Results:          results,
		UnmappedTables:   unmappedTables,
		MissingResources: missingResources,
	}, nil
}

func mapLegacyBoxID(row accessPreparedRow, boxIDMap map[int]int) (*int, bool) {
	legacyBoxID, hasBoxID := valueInt(row.Values, "boxId")
	if !hasBoxID {
		return nil, true
	}
	mapped, found := boxIDMap[legacyBoxID]
	if !found {
		return nil, false
	}
	return &mapped, true
}

func valueString(values map[string]any, key string) string {
	raw, ok := values[key]
	if !ok || raw == nil {
		return ""
	}
	s, ok := raw.(string)
	if !ok {
		return ""
	}
	return s
}

func valueBool(values map[string]any, key string) bool {
	raw, ok := values[key]
	if !ok || raw == nil {
		return false
	}
	b, ok := raw.(bool)
	if !ok {
		return false
	}
	return b
}

func valueInt(values map[string]any, key string) (int, bool) {
	raw, ok := values[key]
	if !ok || raw == nil {
		return 0, false
	}
	v, ok := raw.(int)
	if !ok {
		return 0, false
	}
	return v, true
}

func ensureMDBTools() error {
	if _, err := exec.LookPath("mdb-tables"); err != nil {
		return fmt.Errorf("mdb-tables not found; install mdbtools on the API host")
	}
	if _, err := exec.LookPath("mdb-export"); err != nil {
		return fmt.Errorf("mdb-export not found; install mdbtools on the API host")
	}
	return nil
}

func listAccessTables(ctx context.Context, filePath string) ([]string, error) {
	cmd := exec.CommandContext(ctx, "mdb-tables", "-1", filePath)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			msg = err.Error()
		}
		return nil, fmt.Errorf("%w: failed listing access tables: %s", ErrInvalidInput, msg)
	}

	lines := strings.Split(strings.ReplaceAll(string(out), "\r\n", "\n"), "\n")
	var tables []string
	for _, line := range lines {
		name := strings.TrimSpace(line)
		if name == "" {
			continue
		}
		tables = append(tables, name)
	}
	return tables, nil
}

func exportAccessTable(ctx context.Context, filePath, tableName string) (accessTable, error) {
	cmd := exec.CommandContext(
		ctx,
		"mdb-export",
		"-H",
		"-D",
		"%Y-%m-%d",
		"-d",
		",",
		"-q",
		"\"",
		filePath,
		tableName,
	)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			msg = err.Error()
		}
		return accessTable{}, fmt.Errorf("export table %q: %s", tableName, msg)
	}

	reader := csv.NewReader(bytes.NewReader(out))
	reader.FieldsPerRecord = -1
	reader.TrimLeadingSpace = true
	reader.LazyQuotes = true
	records, err := reader.ReadAll()
	if err != nil {
		return accessTable{}, fmt.Errorf("parse csv for table %q: %w", tableName, err)
	}
	if len(records) == 0 {
		return accessTable{}, fmt.Errorf("table %q has no rows", tableName)
	}

	headers := make([]string, len(records[0]))
	copy(headers, records[0])
	rows := make([][]string, 0, len(records)-1)
	for i := 1; i < len(records); i++ {
		rows = append(rows, records[i])
	}

	return accessTable{
		Name:    tableName,
		Headers: headers,
		Rows:    rows,
	}, nil
}

func matchAccessTables(exports []accessTable) map[string]accessTable {
	selected := map[string]accessTable{}
	used := map[string]bool{}

	for _, spec := range accessImportSpecs {
		bestScore := -1
		var best accessTable
		for _, table := range exports {
			if used[table.Name] {
				continue
			}
			score, ok := scoreAccessTable(spec, table)
			if !ok {
				continue
			}
			if score > bestScore {
				bestScore = score
				best = table
			}
		}
		if bestScore >= 0 {
			selected[spec.Resource] = best
			used[best.Name] = true
		}
	}

	return selected
}

func scoreAccessTable(spec accessSpec, table accessTable) (int, bool) {
	lookup := specLookup(spec)
	present := map[string]bool{}
	for _, header := range table.Headers {
		if canonical, ok := lookup[normalizeAccessKey(header)]; ok {
			present[canonical] = true
		}
	}

	requiredMatched := 0
	for _, required := range spec.RequiredKeys {
		if present[required] {
			requiredMatched++
		}
	}
	if requiredMatched != len(spec.RequiredKeys) {
		return 0, false
	}

	optionalMatched := 0
	for _, optional := range spec.OptionalKeys {
		if present[optional] {
			optionalMatched++
		}
	}

	score := requiredMatched*25 + optionalMatched*3
	normalizedTableName := normalizeAccessKey(table.Name)
	for _, kw := range spec.Keywords {
		if strings.Contains(normalizedTableName, normalizeAccessKey(kw)) {
			score += 100
			break
		}
	}
	return score, true
}

func prepareRows(spec accessSpec, table accessTable) ([]accessPreparedRow, []string) {
	lookup := specLookup(spec)
	normalizedHeaders := make([]string, len(table.Headers))
	for i, header := range table.Headers {
		normalizedHeaders[i] = normalizeAccessKey(header)
	}

	var rows []accessPreparedRow
	var errors []string
	for i, row := range table.Rows {
		rowNumber := i + 2 // +1 header, +1 1-based

		rawByHeader := map[string]string{}
		values := map[string]any{}

		for col := 0; col < len(normalizedHeaders); col++ {
			value := ""
			if col < len(row) {
				value = strings.TrimSpace(row[col])
			}
			header := normalizedHeaders[col]
			rawByHeader[header] = value

			canonical, ok := lookup[header]
			if !ok {
				continue
			}
			switch canonical {
			case "boxId", "storageId":
				if value == "" {
					values[canonical] = nil
					continue
				}
				parsed, err := strconv.Atoi(value)
				if err != nil {
					errors = append(errors, fmt.Sprintf("row %d: invalid %s value %q", rowNumber, canonical, value))
					continue
				}
				values[canonical] = parsed
			case "isDepleted":
				values[canonical] = parseBoolLike(value)
			case "expiryDate":
				values[canonical] = normalizeDate(value)
			default:
				values[canonical] = value
			}
		}

		if isEmptyPreparedValues(values) {
			continue
		}

		missing := missingRequiredValues(spec.RequiredKeys, values)
		if len(missing) > 0 {
			errors = append(errors, fmt.Sprintf("row %d: missing %s", rowNumber, strings.Join(missing, ", ")))
			continue
		}

		legacyID := extractLegacyID(spec, rawByHeader)
		rows = append(rows, accessPreparedRow{
			RowNumber: rowNumber,
			Values:    values,
			LegacyID:  legacyID,
		})
	}

	return rows, errors
}

func specLookup(spec accessSpec) map[string]string {
	lookup := map[string]string{}
	for _, key := range spec.RequiredKeys {
		lookup[normalizeAccessKey(key)] = key
	}
	for _, key := range spec.OptionalKeys {
		lookup[normalizeAccessKey(key)] = key
	}
	for alias, canonical := range spec.Aliases {
		lookup[normalizeAccessKey(alias)] = canonical
	}
	return lookup
}

func missingRequiredValues(required []string, values map[string]any) []string {
	var missing []string
	for _, key := range required {
		raw, ok := values[key]
		if !ok || raw == nil {
			missing = append(missing, key)
			continue
		}
		if s, ok := raw.(string); ok && strings.TrimSpace(s) == "" {
			missing = append(missing, key)
		}
	}
	return missing
}

func isEmptyPreparedValues(values map[string]any) bool {
	if len(values) == 0 {
		return true
	}
	for _, raw := range values {
		if raw == nil {
			continue
		}
		if s, ok := raw.(string); ok {
			if strings.TrimSpace(s) == "" {
				continue
			}
		}
		return false
	}
	return true
}

func extractLegacyID(spec accessSpec, rawByHeader map[string]string) *int {
	candidates := make([]string, 0, len(spec.LegacyIDAliases)+1)
	candidates = append(candidates, "id")
	candidates = append(candidates, spec.LegacyIDAliases...)
	for _, candidate := range candidates {
		raw, ok := rawByHeader[normalizeAccessKey(candidate)]
		if !ok || strings.TrimSpace(raw) == "" {
			continue
		}
		parsed, err := strconv.Atoi(strings.TrimSpace(raw))
		if err == nil {
			return &parsed
		}
	}
	return nil
}

func normalizeDate(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	formats := []string{
		"2006-01-02",
		"2006/01/02",
		"2006-01-02 15:04:05",
		"2006-01-02 15:04:05Z07:00",
		"01/02/2006",
		"1/2/2006",
		"01-02-2006",
		"1-2-2006",
		time.RFC3339,
	}
	for _, format := range formats {
		parsed, err := time.Parse(format, value)
		if err == nil {
			return parsed.Format("2006-01-02")
		}
	}
	return value
}

func parseBoolLike(value string) bool {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "1", "true", "t", "yes", "y":
		return true
	default:
		return false
	}
}

func normalizeAccessKey(value string) string {
	if value == "" {
		return ""
	}
	var b strings.Builder
	b.Grow(len(value))
	for _, r := range value {
		switch {
		case r >= 'a' && r <= 'z':
			b.WriteRune(r)
		case r >= 'A' && r <= 'Z':
			b.WriteRune(r + 32)
		case r >= '0' && r <= '9':
			b.WriteRune(r)
		}
	}
	return b.String()
}

func sortStrings(items []string) {
	if len(items) < 2 {
		return
	}
	for i := 0; i < len(items)-1; i++ {
		for j := i + 1; j < len(items); j++ {
			if items[j] < items[i] {
				items[i], items[j] = items[j], items[i]
			}
		}
	}
}
