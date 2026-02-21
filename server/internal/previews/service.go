package previews

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/base64"
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/jpeg"
	"image/png"
	"strings"
	"time"

	_ "image/gif"

	internaldb "github.com/mjhen/elnote/server/internal/db"
)

var (
	ErrForbidden    = errors.New("forbidden")
	ErrNotFound     = errors.New("not found")
	ErrInvalidInput = errors.New("invalid input")
)

const (
	thumbnailMaxDim = 256
)

// ---------------------------------------------------------------------------
// Types
// ---------------------------------------------------------------------------

type Preview struct {
	ID           string    `json:"previewId"`
	AttachmentID string    `json:"attachmentId"`
	PreviewType  string    `json:"previewType"`
	MimeType     string    `json:"mimeType"`
	Width        int       `json:"width"`
	Height       int       `json:"height"`
	DataBase64   string    `json:"dataBase64"`
	CreatedAt    time.Time `json:"createdAt"`
}

type GenerateInput struct {
	AttachmentID string
	ImageData    []byte
	SourceMime   string
	ActorUserID  string
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

// GenerateThumbnail creates a thumbnail preview from raw image data
// and stores it in the attachment_previews table.
func (s *Service) GenerateThumbnail(ctx context.Context, in GenerateInput) (*Preview, error) {
	if len(in.ImageData) == 0 {
		return nil, fmt.Errorf("%w: image data is empty", ErrInvalidInput)
	}

	// Decode source image
	img, format, err := image.Decode(bytes.NewReader(in.ImageData))
	if err != nil {
		return nil, fmt.Errorf("%w: cannot decode image: %v", ErrInvalidInput, err)
	}
	_ = format

	// Generate thumbnail (simple nearest-neighbor downscale)
	thumb := resizeToFit(img, thumbnailMaxDim, thumbnailMaxDim)

	// Encode as PNG
	var buf bytes.Buffer
	if err := png.Encode(&buf, thumb); err != nil {
		return nil, fmt.Errorf("encode thumbnail: %w", err)
	}

	bounds := thumb.Bounds()
	width := bounds.Dx()
	height := bounds.Dy()

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback()

	var preview Preview
	err = tx.QueryRowContext(ctx,
		`INSERT INTO attachment_previews (attachment_id, preview_type, mime_type, width, height, data)
		 VALUES ($1, 'thumbnail', 'image/png', $2, $3, $4)
		 ON CONFLICT (attachment_id, preview_type) DO NOTHING
		 RETURNING id, attachment_id, preview_type, mime_type, width, height, created_at`,
		in.AttachmentID, width, height, buf.Bytes(),
	).Scan(&preview.ID, &preview.AttachmentID, &preview.PreviewType, &preview.MimeType, &preview.Width, &preview.Height, &preview.CreatedAt)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			// Already exists (DO NOTHING triggered), fetch existing
			return s.GetPreview(ctx, in.AttachmentID, "thumbnail")
		}
		return nil, fmt.Errorf("insert preview: %w", err)
	}
	preview.DataBase64 = base64.StdEncoding.EncodeToString(buf.Bytes())

	internaldb.AppendAuditEvent(ctx, tx, in.ActorUserID, "attachment.preview_generated", "attachment", in.AttachmentID, nil)

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit: %w", err)
	}

	return &preview, nil
}

// GetPreview retrieves a stored preview by attachment ID and type.
func (s *Service) GetPreview(ctx context.Context, attachmentID, previewType string) (*Preview, error) {
	var preview Preview
	var data []byte
	err := s.db.QueryRowContext(ctx,
		`SELECT id, attachment_id, preview_type, mime_type, width, height, data, created_at
		 FROM attachment_previews
		 WHERE attachment_id = $1 AND preview_type = $2`,
		attachmentID, previewType,
	).Scan(&preview.ID, &preview.AttachmentID, &preview.PreviewType, &preview.MimeType, &preview.Width, &preview.Height, &data, &preview.CreatedAt)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("query preview: %w", err)
	}
	preview.DataBase64 = base64.StdEncoding.EncodeToString(data)
	return &preview, nil
}

// GetPreviewForAttachment retrieves the thumbnail for an attachment,
// checking experiment access first.
func (s *Service) GetPreviewForAttachment(ctx context.Context, attachmentID, userID, role string) (*Preview, error) {
	// Verify access
	var expOwner, expStatus string
	err := s.db.QueryRowContext(ctx,
		`SELECT e.owner_user_id, e.status
		 FROM attachments a
		 JOIN experiments e ON e.id = a.experiment_id
		 WHERE a.id = $1`,
		attachmentID,
	).Scan(&expOwner, &expStatus)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("query attachment: %w", err)
	}
	if expOwner != userID {
		if role != "admin" || expStatus != "completed" {
			return nil, ErrForbidden
		}
	}

	return s.GetPreview(ctx, attachmentID, "thumbnail")
}

// ListPreviewsForExperiment returns all previews for attachments of an experiment.
func (s *Service) ListPreviewsForExperiment(ctx context.Context, experimentID, userID, role string) ([]Preview, error) {
	// Access check
	var expOwner, expStatus string
	err := s.db.QueryRowContext(ctx,
		`SELECT owner_user_id, status FROM experiments WHERE id = $1`, experimentID,
	).Scan(&expOwner, &expStatus)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("query experiment: %w", err)
	}
	if expOwner != userID {
		if role != "admin" || expStatus != "completed" {
			return nil, ErrForbidden
		}
	}

	rows, err := s.db.QueryContext(ctx,
		`SELECT ap.id, ap.attachment_id, ap.preview_type, ap.mime_type, ap.width, ap.height, ap.data, ap.created_at
		 FROM attachment_previews ap
		 JOIN attachments a ON a.id = ap.attachment_id
		 WHERE a.experiment_id = $1
		 ORDER BY ap.created_at`,
		experimentID,
	)
	if err != nil {
		return nil, fmt.Errorf("query previews: %w", err)
	}
	defer rows.Close()

	var previews []Preview
	for rows.Next() {
		var p Preview
		var data []byte
		if err := rows.Scan(&p.ID, &p.AttachmentID, &p.PreviewType, &p.MimeType, &p.Width, &p.Height, &data, &p.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan preview: %w", err)
		}
		p.DataBase64 = base64.StdEncoding.EncodeToString(data)
		previews = append(previews, p)
	}
	if previews == nil {
		previews = []Preview{}
	}
	return previews, nil
}

// IsSupportedImageMime returns true if the MIME type is an image we can thumbnail.
func IsSupportedImageMime(mime string) bool {
	lower := strings.ToLower(mime)
	return lower == "image/png" || lower == "image/jpeg" || lower == "image/gif" || lower == "image/jpg"
}

// EncodeJPEG is a utility for callers that need JPEG output.
func EncodeJPEG(img image.Image, quality int) ([]byte, error) {
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, img, &jpeg.Options{Quality: quality}); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// resizeToFit downscales an image to fit within maxW x maxH, preserving aspect ratio.
// Uses simple nearest-neighbor sampling for performance.
func resizeToFit(src image.Image, maxW, maxH int) image.Image {
	bounds := src.Bounds()
	srcW := bounds.Dx()
	srcH := bounds.Dy()

	if srcW <= maxW && srcH <= maxH {
		return src
	}

	// Calculate scale
	scaleX := float64(maxW) / float64(srcW)
	scaleY := float64(maxH) / float64(srcH)
	scale := scaleX
	if scaleY < scaleX {
		scale = scaleY
	}

	dstW := int(float64(srcW) * scale)
	dstH := int(float64(srcH) * scale)
	if dstW < 1 {
		dstW = 1
	}
	if dstH < 1 {
		dstH = 1
	}

	dst := image.NewRGBA(image.Rect(0, 0, dstW, dstH))

	// Fill with dark background (like BioArena's thumbnail canvas)
	draw.Draw(dst, dst.Bounds(), &image.Uniform{color.RGBA{0x1f, 0x1f, 0x1f, 0xff}}, image.Point{}, draw.Src)

	// Nearest-neighbor resize
	for y := 0; y < dstH; y++ {
		srcY := y * srcH / dstH
		for x := 0; x < dstW; x++ {
			srcX := x * srcW / dstW
			dst.Set(x, y, src.At(bounds.Min.X+srcX, bounds.Min.Y+srcY))
		}
	}

	return dst
}
