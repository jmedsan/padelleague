package handlers

import (
	"bytes"
	"image"
	"image/color"
	_ "image/gif"
	"image/jpeg"
	_ "image/png"
	"io"
	"log/slog"
	"mime/multipart"
	"strings"

	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tools/filesystem"
	"github.com/rwcarlsen/goexif/exif"
	"golang.org/x/image/draw"
	_ "golang.org/x/image/webp"

	"padelleague/league"
)

const (
	avatarMaxDim        = 400
	avatarMaxUploadSize = 5 << 20 // matches the users.avatar FileField MaxSize
	avatarMaxSourceDim  = 8000    // reject implausibly large source images before full decode
)

// PlayerAvatarUpload handles POST to upload and set a player's own avatar
// photo. Only the player themselves may set their avatar. The image is
// corrected for EXIF orientation, center-cropped to a square, resized to
// avatarMaxDim x avatarMaxDim, and re-encoded as JPEG before being saved on
// the record.
func (h *PlayerHandler) PlayerAvatarUpload(e *core.RequestEvent) error {
	id := e.Request.PathValue("id")
	if e.Auth == nil || e.Auth.Id != id {
		return alertError(e, "No puedes cambiar la foto de otro jugador")
	}

	user, err := h.app.FindRecordById("users", id)
	if err != nil {
		return alertError(e, "Jugador no encontrado")
	}

	fh := fileHeader(e, "avatar")
	if fh == nil {
		return alertError(e, "Selecciona una imagen")
	}

	if !strings.HasPrefix(fh.Header.Get("Content-Type"), "image/") {
		return alertError(e, "El archivo debe ser una imagen")
	}

	if fh.Size > avatarMaxUploadSize {
		return alertError(e, "La imagen no puede superar los 5 MB")
	}

	f, errMsg := compressAvatar(fh, id)
	if errMsg != "" {
		return alertError(e, errMsg)
	}

	user.Set("avatar", f)
	if err := h.app.Save(user); err != nil {
		slog.Error("save avatar", "err", err)
		return alertError(e, "Error al guardar la foto")
	}

	return h.render.Partial(e, "avatar-fragment.html", map[string]any{
		"Mode":          PlayerFull,
		"ID":            user.Id,
		"Name":          user.GetString("display_name"),
		"Roles":         user.GetStringSlice("roles"),
		"AvatarURL":     league.AvatarURL(user.Id, user.GetString("avatar")),
		"CanEditAvatar": true,
	})
}

// compressAvatar decodes the uploaded image, corrects EXIF orientation,
// center-crops it to a square, resizes it to fit within avatarMaxDim x
// avatarMaxDim, and re-encodes it as a JPEG file ready to save on a record.
// On failure it returns a user-facing Spanish message instead of a raw error.
func compressAvatar(fh *multipart.FileHeader, userID string) (*filesystem.File, string) {
	src, err := fh.Open()
	if err != nil {
		return nil, "No se pudo leer la imagen"
	}
	defer func() { _ = src.Close() }()

	cfg, _, err := image.DecodeConfig(src)
	if err != nil {
		return nil, "Imagen no válida"
	}
	if cfg.Width > avatarMaxSourceDim || cfg.Height > avatarMaxSourceDim {
		return nil, "La imagen es demasiado grande"
	}

	if _, err := src.Seek(0, 0); err != nil {
		return nil, "No se pudo leer la imagen"
	}
	orientation := readOrientation(src)

	if _, err := src.Seek(0, 0); err != nil {
		return nil, "No se pudo leer la imagen"
	}
	img, _, err := image.Decode(src)
	if err != nil {
		return nil, "Imagen no válida"
	}

	img = applyOrientation(img, orientation)
	square := cropToSquare(img)
	resized := resizeToFit(square, avatarMaxDim, avatarMaxDim)
	opaque := flattenOnWhite(resized)

	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, opaque, &jpeg.Options{Quality: 80}); err != nil {
		slog.Error("encode avatar", "err", err)
		return nil, "Error al procesar la imagen"
	}

	f, err := filesystem.NewFileFromBytes(buf.Bytes(), userID+"_avatar.jpg")
	if err != nil {
		slog.Error("build avatar file", "err", err)
		return nil, "Error al procesar la imagen"
	}
	return f, ""
}

// readOrientation reads the EXIF orientation tag (1-8) from r, defaulting to
// 1 (no transform needed) when EXIF data is absent or unreadable.
func readOrientation(r io.Reader) int {
	x, err := exif.Decode(r)
	if err != nil {
		return 1
	}
	tag, err := x.Get(exif.Orientation)
	if err != nil {
		return 1
	}
	o, err := tag.Int(0)
	if err != nil {
		return 1
	}
	return o
}

// applyOrientation rotates/flips img according to the EXIF orientation value
// (1-8, per the EXIF 2.2 spec) so it displays upright regardless of how the
// capturing device wrote it.
func applyOrientation(img image.Image, orientation int) image.Image {
	switch orientation {
	case 2:
		return flipH(img)
	case 3:
		return rotate180(img)
	case 4:
		return flipV(img)
	case 5:
		return flipH(rotate90(img))
	case 6:
		return rotate90(img)
	case 7:
		return flipH(rotate270(img))
	case 8:
		return rotate270(img)
	default:
		return img
	}
}

func rotate90(img image.Image) image.Image {
	b := img.Bounds()
	dst := image.NewRGBA(image.Rect(0, 0, b.Dy(), b.Dx()))
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			dst.Set(b.Max.Y-1-y, x, img.At(x, y))
		}
	}
	return dst
}

func rotate180(img image.Image) image.Image {
	b := img.Bounds()
	dst := image.NewRGBA(b)
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			dst.Set(b.Max.X-1-x, b.Max.Y-1-y, img.At(x, y))
		}
	}
	return dst
}

func rotate270(img image.Image) image.Image {
	b := img.Bounds()
	dst := image.NewRGBA(image.Rect(0, 0, b.Dy(), b.Dx()))
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			dst.Set(y, b.Max.X-1-x, img.At(x, y))
		}
	}
	return dst
}

func flipH(img image.Image) image.Image {
	b := img.Bounds()
	dst := image.NewRGBA(b)
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			dst.Set(b.Max.X-1-x, y, img.At(x, y))
		}
	}
	return dst
}

func flipV(img image.Image) image.Image {
	b := img.Bounds()
	dst := image.NewRGBA(b)
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			dst.Set(x, b.Max.Y-1-y, img.At(x, y))
		}
	}
	return dst
}

// cropToSquare center-crops img to a square using the shorter side.
func cropToSquare(img image.Image) image.Image {
	b := img.Bounds()
	w, h := b.Dx(), b.Dy()
	if w == h {
		return img
	}
	side := w
	if h < side {
		side = h
	}
	x0 := b.Min.X + (w-side)/2
	y0 := b.Min.Y + (h-side)/2
	rect := image.Rect(0, 0, side, side)
	dst := image.NewRGBA(rect)
	draw.Draw(dst, rect, img, image.Pt(x0, y0), draw.Src)
	return dst
}

// resizeToFit scales img down to fit within maxW x maxH, preserving aspect
// ratio. Images already within bounds are returned unchanged.
func resizeToFit(img image.Image, maxW, maxH int) image.Image {
	b := img.Bounds()
	w, h := b.Dx(), b.Dy()
	if w <= maxW && h <= maxH {
		return img
	}

	scale := float64(maxW) / float64(w)
	if hs := float64(maxH) / float64(h); hs < scale {
		scale = hs
	}
	nw := int(float64(w) * scale)
	nh := int(float64(h) * scale)
	if nw < 1 {
		nw = 1
	}
	if nh < 1 {
		nh = 1
	}

	dst := image.NewRGBA(image.Rect(0, 0, nw, nh))
	draw.CatmullRom.Scale(dst, dst.Bounds(), img, b, draw.Over, nil)
	return dst
}

// flattenOnWhite draws img onto an opaque white background, so images with
// transparency (PNG, GIF, WebP) don't turn black when encoded as JPEG.
func flattenOnWhite(img image.Image) image.Image {
	b := img.Bounds()
	dst := image.NewRGBA(b)
	draw.Draw(dst, b, image.NewUniform(color.White), image.Point{}, draw.Src)
	draw.Draw(dst, b, img, b.Min, draw.Over)
	return dst
}
