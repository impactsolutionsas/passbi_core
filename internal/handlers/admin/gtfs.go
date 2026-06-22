package admin

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"mime/multipart"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/passbi/passbi_core/internal/middleware"
)

// ─────────────────── types ───────────────────

type agencyRow struct {
	ID            string      `json:"id"`
	Label         string      `json:"label"`
	Country       string      `json:"country"`
	City          string      `json:"city"`
	IsActive      bool        `json:"is_active"`
	Notes         *string     `json:"notes,omitempty"`
	CreatedAt     time.Time   `json:"created_at"`
	VersionsCount int         `json:"versions_count"`
	LatestVersion *versionRow `json:"latest_version,omitempty"`
	LastImport    *importLog  `json:"last_import,omitempty"`
	IsRunning     bool        `json:"is_running"`
}

type versionRow struct {
	ID            int64     `json:"id"`
	AgencyID      string    `json:"agency_id"`
	VersionNumber int       `json:"version_number"`
	Filename      string    `json:"filename"`
	FileSizeBytes *int64    `json:"file_size_bytes,omitempty"`
	SHA256        *string   `json:"sha256,omitempty"`
	UploadedBy    *string   `json:"uploaded_by,omitempty"`
	UploadedAt    time.Time `json:"uploaded_at"`
	Notes         *string   `json:"notes,omitempty"`
	Status        string    `json:"status"`
	ImportLogID   *int64    `json:"import_log_id,omitempty"`
}

type importLog struct {
	ID          int64      `json:"id"`
	AgencyID    string     `json:"agency_id"`
	Status      string     `json:"status"`
	StartedAt   time.Time  `json:"started_at"`
	CompletedAt *time.Time `json:"completed_at"`
	StopsCount  int        `json:"stops_count"`
	RoutesCount int        `json:"routes_count"`
	NodesCount  int        `json:"nodes_count"`
	EdgesCount  int        `json:"edges_count"`
	ErrMsg      *string    `json:"error_message"`
	VersionID   *int64     `json:"gtfs_version_id,omitempty"`
}

// ─────────────────── env helpers ───────────────────

func getGTFSFolder() string {
	if p := os.Getenv("GTFS_FOLDER"); p != "" {
		return p
	}
	return "./gtfs_folder"
}

func getImporterBin() string {
	if p := os.Getenv("IMPORTER_BIN"); p != "" {
		return p
	}
	return "./bin/passbi-import"
}

// ─────────────────── ListAgencies ───────────────────

// ListAgencies GET /api/admin/gtfs/agencies
func ListAgencies(db *pgxpool.Pool) fiber.Handler {
	return func(c *fiber.Ctx) error {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		rows, err := db.Query(ctx, `
			SELECT a.id, a.label, a.country, a.city, a.is_active, a.notes, a.created_at,
			       COUNT(v.id) AS versions_count
			FROM gtfs_agency a
			LEFT JOIN gtfs_version v ON v.agency_id = a.id
			GROUP BY a.id
			ORDER BY a.country, a.city, a.label
		`)
		if err != nil {
			return c.Status(500).JSON(fiber.Map{"error": err.Error()})
		}
		defer rows.Close()

		agencies := []agencyRow{}
		for rows.Next() {
			var a agencyRow
			if err := rows.Scan(&a.ID, &a.Label, &a.Country, &a.City, &a.IsActive,
				&a.Notes, &a.CreatedAt, &a.VersionsCount); err != nil {
				continue
			}
			agencies = append(agencies, a)
		}
		rows.Close()

		for i := range agencies {
			enrichAgency(ctx, db, &agencies[i])
		}

		return c.JSON(agencies)
	}
}

func enrichAgency(ctx context.Context, db *pgxpool.Pool, a *agencyRow) {
	// Latest version
	v := &versionRow{}
	err := db.QueryRow(ctx, `
		SELECT id, agency_id, version_number, filename, file_size_bytes, sha256,
		       uploaded_by, uploaded_at, notes, status, import_log_id
		FROM gtfs_version
		WHERE agency_id = $1
		ORDER BY version_number DESC
		LIMIT 1
	`, a.ID).Scan(&v.ID, &v.AgencyID, &v.VersionNumber, &v.Filename, &v.FileSizeBytes, &v.SHA256,
		&v.UploadedBy, &v.UploadedAt, &v.Notes, &v.Status, &v.ImportLogID)
	if err == nil {
		a.LatestVersion = v
	}

	// Last import log
	il := &importLog{}
	err = db.QueryRow(ctx, `
		SELECT id, agency_id, status, started_at, completed_at,
		       stops_count, routes_count, nodes_count, edges_count, error_message, gtfs_version_id
		FROM import_log
		WHERE agency_id = $1
		ORDER BY started_at DESC
		LIMIT 1
	`, a.ID).Scan(&il.ID, &il.AgencyID, &il.Status, &il.StartedAt, &il.CompletedAt,
		&il.StopsCount, &il.RoutesCount, &il.NodesCount, &il.EdgesCount, &il.ErrMsg, &il.VersionID)
	if err == nil {
		a.LastImport = il
		a.IsRunning = il.Status == "running"
	}
}

// ─────────────────── CreateAgency ───────────────────

type createAgencyReq struct {
	ID      string  `json:"id"`
	Label   string  `json:"label"`
	Country string  `json:"country"`
	City    string  `json:"city"`
	Notes   *string `json:"notes"`
}

// CreateAgency POST /api/admin/gtfs/agencies
func CreateAgency(db *pgxpool.Pool) fiber.Handler {
	return func(c *fiber.Ctx) error {
		var req createAgencyReq
		if err := c.BodyParser(&req); err != nil {
			return c.Status(400).JSON(fiber.Map{"error": "invalid_body"})
		}
		if req.ID == "" || req.Label == "" || req.Country == "" || req.City == "" {
			return c.Status(400).JSON(fiber.Map{"error": "id, label, country and city are required"})
		}

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		var a agencyRow
		err := db.QueryRow(ctx, `
			INSERT INTO gtfs_agency (id, label, country, city, notes)
			VALUES ($1, $2, $3, $4, $5)
			RETURNING id, label, country, city, is_active, notes, created_at
		`, req.ID, req.Label, req.Country, req.City, req.Notes).Scan(
			&a.ID, &a.Label, &a.Country, &a.City, &a.IsActive, &a.Notes, &a.CreatedAt,
		)
		if err != nil {
			if strings.Contains(err.Error(), "duplicate key") {
				return c.Status(409).JSON(fiber.Map{"error": "agency_id_already_exists"})
			}
			return c.Status(500).JSON(fiber.Map{"error": err.Error()})
		}

		return c.Status(201).JSON(a)
	}
}

// ─────────────────── UpdateAgency ───────────────────

type updateAgencyReq struct {
	Label    *string `json:"label"`
	Country  *string `json:"country"`
	City     *string `json:"city"`
	IsActive *bool   `json:"is_active"`
	Notes    *string `json:"notes"`
}

// UpdateAgency PATCH /api/admin/gtfs/agencies/:id
func UpdateAgency(db *pgxpool.Pool) fiber.Handler {
	return func(c *fiber.Ctx) error {
		agencyID := c.Params("id")
		var req updateAgencyReq
		if err := c.BodyParser(&req); err != nil {
			return c.Status(400).JSON(fiber.Map{"error": "invalid_body"})
		}

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		_, err := db.Exec(ctx, `
			UPDATE gtfs_agency
			SET label     = COALESCE($2, label),
			    country   = COALESCE($3, country),
			    city      = COALESCE($4, city),
			    is_active = COALESCE($5, is_active),
			    notes     = COALESCE($6, notes)
			WHERE id = $1
		`, agencyID, req.Label, req.Country, req.City, req.IsActive, req.Notes)
		if err != nil {
			return c.Status(500).JSON(fiber.Map{"error": err.Error()})
		}

		return c.JSON(fiber.Map{"ok": true})
	}
}

// ─────────────────── UploadGTFS ───────────────────

// UploadGTFS POST /api/admin/gtfs/agencies/:agencyId/upload
func UploadGTFS(db *pgxpool.Pool) fiber.Handler {
	return func(c *fiber.Ctx) error {
		agencyID := c.Params("agencyId")

		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		var exists bool
		_ = db.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM gtfs_agency WHERE id = $1)`, agencyID).Scan(&exists)
		if !exists {
			return c.Status(404).JSON(fiber.Map{"error": "agency_not_found"})
		}

		fh, err := c.FormFile("file")
		if err != nil {
			return c.Status(400).JSON(fiber.Map{"error": "file_required"})
		}
		if !strings.HasSuffix(strings.ToLower(fh.Filename), ".zip") {
			return c.Status(400).JSON(fiber.Map{"error": "only_zip_files_accepted"})
		}
		const maxSize = 200 * 1024 * 1024 // 200 MB
		if fh.Size > maxSize {
			return c.Status(400).JSON(fiber.Map{"error": "file_too_large", "max_bytes": maxSize})
		}

		// Next version number (per agency)
		var nextVersion int
		_ = db.QueryRow(ctx,
			`SELECT COALESCE(MAX(version_number), 0) + 1 FROM gtfs_version WHERE agency_id = $1`,
			agencyID,
		).Scan(&nextVersion)

		// Storage path: {GTFS_FOLDER}/{agencyId}/v{N}_{timestamp}.zip
		gtfsFolder := getGTFSFolder()
		agencyDir := filepath.Join(gtfsFolder, agencyID)
		if err := os.MkdirAll(agencyDir, 0o755); err != nil {
			return c.Status(500).JSON(fiber.Map{"error": "cannot_create_directory"})
		}

		ts := time.Now().UTC().Format("20060102_150405")
		storageName := fmt.Sprintf("v%d_%s.zip", nextVersion, ts)
		storagePath := filepath.Join(agencyDir, storageName)

		hash, size, err := saveFileWithHash(fh, storagePath)
		if err != nil {
			return c.Status(500).JSON(fiber.Map{"error": "save_failed", "detail": err.Error()})
		}

		uploadedBy := "unknown"
		if admin, ok := c.Locals("admin").(*middleware.AdminContext); ok {
			uploadedBy = admin.Email
		}

		notes := c.FormValue("notes")

		var v versionRow
		err = db.QueryRow(ctx, `
			INSERT INTO gtfs_version
				(agency_id, version_number, filename, storage_path, file_size_bytes, sha256, uploaded_by, notes)
			VALUES ($1, $2, $3, $4, $5, $6, $7, NULLIF($8, ''))
			RETURNING id, agency_id, version_number, filename, file_size_bytes, sha256,
			          uploaded_by, uploaded_at, notes, status, import_log_id
		`, agencyID, nextVersion, fh.Filename, storagePath, size, hash, uploadedBy, notes).Scan(
			&v.ID, &v.AgencyID, &v.VersionNumber, &v.Filename, &v.FileSizeBytes, &v.SHA256,
			&v.UploadedBy, &v.UploadedAt, &v.Notes, &v.Status, &v.ImportLogID,
		)
		if err != nil {
			_ = os.Remove(storagePath)
			return c.Status(500).JSON(fiber.Map{"error": err.Error()})
		}

		return c.Status(201).JSON(v)
	}
}

// saveFileWithHash streams the multipart file to dst, returning SHA256 hex and actual byte count.
func saveFileWithHash(fh *multipart.FileHeader, dst string) (hashHex string, size int64, err error) {
	src, err := fh.Open()
	if err != nil {
		return "", 0, err
	}
	defer src.Close()

	out, err := os.Create(dst)
	if err != nil {
		return "", 0, err
	}
	defer out.Close()

	h := sha256.New()
	size, err = io.Copy(io.MultiWriter(out, h), src)
	if err != nil {
		return "", 0, err
	}

	return hex.EncodeToString(h.Sum(nil)), size, nil
}

// ─────────────────── ListVersions ───────────────────

// ListVersions GET /api/admin/gtfs/agencies/:agencyId/versions
func ListVersions(db *pgxpool.Pool) fiber.Handler {
	return func(c *fiber.Ctx) error {
		agencyID := c.Params("agencyId")

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		rows, err := db.Query(ctx, `
			SELECT id, agency_id, version_number, filename, file_size_bytes, sha256,
			       uploaded_by, uploaded_at, notes, status, import_log_id
			FROM gtfs_version
			WHERE agency_id = $1
			ORDER BY version_number DESC
		`, agencyID)
		if err != nil {
			return c.Status(500).JSON(fiber.Map{"error": err.Error()})
		}
		defer rows.Close()

		versions := []versionRow{}
		for rows.Next() {
			var v versionRow
			_ = rows.Scan(&v.ID, &v.AgencyID, &v.VersionNumber, &v.Filename, &v.FileSizeBytes,
				&v.SHA256, &v.UploadedBy, &v.UploadedAt, &v.Notes, &v.Status, &v.ImportLogID)
			versions = append(versions, v)
		}
		return c.JSON(versions)
	}
}

// ─────────────────── ImportVersion ───────────────────

type importVersionReq struct {
	RebuildGraph bool `json:"rebuild_graph"`
}

// ImportVersion POST /api/admin/gtfs/versions/:versionId/import
func ImportVersion(db *pgxpool.Pool) fiber.Handler {
	return func(c *fiber.Ctx) error {
		versionID, err := c.ParamsInt("versionId")
		if err != nil {
			return c.Status(400).JSON(fiber.Map{"error": "invalid_version_id"})
		}

		var req importVersionReq
		_ = c.BodyParser(&req)

		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		var v versionRow
		var storagePath string
		err = db.QueryRow(ctx, `
			SELECT id, agency_id, version_number, filename, storage_path, status
			FROM gtfs_version WHERE id = $1
		`, versionID).Scan(&v.ID, &v.AgencyID, &v.VersionNumber, &v.Filename, &storagePath, &v.Status)
		if err != nil {
			return c.Status(404).JSON(fiber.Map{"error": "version_not_found"})
		}

		if v.Status == "active" {
			return c.Status(409).JSON(fiber.Map{"error": "version_already_active"})
		}

		if _, err := os.Stat(storagePath); err != nil {
			return c.Status(400).JSON(fiber.Map{"error": "zip_file_missing", "path": storagePath})
		}

		var running int
		_ = db.QueryRow(ctx,
			`SELECT COUNT(*) FROM import_log WHERE agency_id = $1 AND status = 'running'`,
			v.AgencyID,
		).Scan(&running)
		if running > 0 {
			return c.Status(409).JSON(fiber.Map{"error": "import_already_running"})
		}

		var logID int64
		err = db.QueryRow(ctx, `
			INSERT INTO import_log (agency_id, status, gtfs_version_id)
			VALUES ($1, 'running', $2) RETURNING id
		`, v.AgencyID, v.ID).Scan(&logID)
		if err != nil {
			return c.Status(500).JSON(fiber.Map{"error": err.Error()})
		}

		_, _ = db.Exec(ctx, `UPDATE gtfs_version SET import_log_id = $1 WHERE id = $2`, logID, v.ID)

		originalStatus := v.Status
		go runImporterAsync(db, v.AgencyID, storagePath, req.RebuildGraph, logID, v.ID, originalStatus)

		return c.Status(202).JSON(fiber.Map{
			"log_id":         logID,
			"version_id":     v.ID,
			"version_number": v.VersionNumber,
			"agency_id":      v.AgencyID,
			"message":        "Import started",
		})
	}
}

// ─────────────────── runImporterAsync ───────────────────

func runImporterAsync(db *pgxpool.Pool, agencyID, zipPath string, rebuildGraph bool, logID, versionID int64, originalStatus string) {
	binPath := getImporterBin()

	args := []string{
		fmt.Sprintf("--agency-id=%s", agencyID),
		fmt.Sprintf("--gtfs=%s", zipPath),
	}
	if rebuildGraph {
		args = append(args, "--rebuild-graph")
	}

	cmd := exec.Command(binPath, args...)
	cmd.Env = os.Environ()
	out, err := cmd.CombinedOutput()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err != nil {
		errMsg := fmt.Sprintf("%v\n%s", err, strings.TrimSpace(string(out)))
		_, _ = db.Exec(ctx, `
			UPDATE import_log SET status = 'failed', completed_at = NOW(), error_message = $2
			WHERE id = $1
		`, logID, errMsg)
		// pending → failed ; archived stays archived (it was a restore attempt)
		if originalStatus == "pending" {
			_, _ = db.Exec(ctx, `UPDATE gtfs_version SET status = 'failed' WHERE id = $1`, versionID)
		}
		return
	}

	// Parse counts from output (best effort)
	var stops, routes, nodes, edges int
	for _, line := range strings.Split(string(out), "\n") {
		fmt.Sscanf(line, "Imported %d stops", &stops)
		fmt.Sscanf(line, "Imported %d routes", &routes)
		if strings.Contains(line, "nodes") {
			fmt.Sscanf(line, "Imported %d nodes, %d edges", &nodes, &edges)
		}
	}

	// Archive previous active version
	_, _ = db.Exec(ctx, `
		UPDATE gtfs_version SET status = 'archived'
		WHERE agency_id = $1 AND status = 'active' AND id != $2
	`, agencyID, versionID)

	_, _ = db.Exec(ctx, `UPDATE gtfs_version SET status = 'active' WHERE id = $1`, versionID)

	_, _ = db.Exec(ctx, `
		UPDATE import_log
		SET status = 'success', completed_at = NOW(),
		    stops_count = $2, routes_count = $3, nodes_count = $4, edges_count = $5
		WHERE id = $1
	`, logID, stops, routes, nodes, edges)
}

// ─────────────────── ListImportLogs ───────────────────

// ListImportLogs GET /api/admin/gtfs/logs?agency_id=&limit=50
func ListImportLogs(db *pgxpool.Pool) fiber.Handler {
	return func(c *fiber.Ctx) error {
		agencyID := c.Query("agency_id")
		limit := min(c.QueryInt("limit", 50), 200)

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		var (
			rows interface {
				Next() bool
				Scan(...any) error
				Close()
			}
			err error
		)

		if agencyID != "" {
			rows, err = db.Query(ctx, `
				SELECT id, agency_id, status, started_at, completed_at,
				       stops_count, routes_count, nodes_count, edges_count, error_message, gtfs_version_id
				FROM import_log
				WHERE agency_id = $1
				ORDER BY started_at DESC
				LIMIT $2
			`, agencyID, limit)
		} else {
			rows, err = db.Query(ctx, `
				SELECT id, agency_id, status, started_at, completed_at,
				       stops_count, routes_count, nodes_count, edges_count, error_message, gtfs_version_id
				FROM import_log
				ORDER BY started_at DESC
				LIMIT $1
			`, limit)
		}
		if err != nil {
			return c.Status(500).JSON(fiber.Map{"error": err.Error()})
		}
		defer rows.Close()

		logs := []importLog{}
		for rows.Next() {
			var l importLog
			_ = rows.Scan(
				&l.ID, &l.AgencyID, &l.Status, &l.StartedAt, &l.CompletedAt,
				&l.StopsCount, &l.RoutesCount, &l.NodesCount, &l.EdgesCount, &l.ErrMsg, &l.VersionID,
			)
			logs = append(logs, l)
		}

		return c.JSON(logs)
	}
}
