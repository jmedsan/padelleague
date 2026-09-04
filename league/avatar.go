package league

import (
	"bytes"
	"errors"
	"image"
	"image/color"
	_ "image/gif"
	"image/jpeg"
	_ "image/png"
	"io"
	"log/slog"

	"github.com/pocketbase/pocketbase/tools/filesystem"
	"github.com/rwcarlsen/goexif/exif"
	"golang.org/x/image/draw"
	_ "golang.org/x/image/webp"
)

const (
	avatarMaxDim          = 400
	avatarMaxSourcePixels = 25_000_000 // reject implausibly large source images before full decode (25 Mpx)
	// avatarPreRotateDim bounds the image BEFORE the pixel-by-pixel rotate/crop
	// passes, so a 12 Mpx phone photo doesn't allocate a full-resolution RGBA
	// buffer plus a rotated copy. Kept well above avatarMaxDim so the later
	// square resize still has enough detail to downsample cleanly.
	avatarPreRotateDim = 800
)

// ErrAvatarUnreadable, ErrAvatarInvalid, and ErrAvatarTooLarge classify
// CompressAvatarBytes failures so callers can map them to user-facing
// messages without depending on error string text.
var (
	ErrAvatarUnreadable = errors.New("avatar: could not read source image")
	ErrAvatarInvalid    = errors.New("avatar: invalid image")
	ErrAvatarTooLarge   = errors.New("avatar: source image too large")
)

// CompressLogoBytes decodes an image from r, corrects EXIF orientation,
// resizes it to fit within 800x800 preserving aspect ratio, and re-encodes
// as JPEG. Unlike CompressAvatarBytes it does NOT crop to square.
func CompressLogoBytes(r io.ReadSeeker, filename string) (*filesystem.File, error) {
	cfg, _, err := image.DecodeConfig(r)
	if err != nil {
		return nil, ErrAvatarInvalid
	}
	if cfg.Width*cfg.Height > avatarMaxSourcePixels {
		return nil, ErrAvatarTooLarge
	}
	if _, err := r.Seek(0, 0); err != nil {
		return nil, ErrAvatarUnreadable
	}
	orientation := readOrientation(r)
	if _, err := r.Seek(0, 0); err != nil {
		return nil, ErrAvatarUnreadable
	}
	img, _, err := image.Decode(r)
	if err != nil {
		return nil, ErrAvatarInvalid
	}
	small := resizeToFit(img, 800, 800)
	oriented := applyOrientation(small, orientation)
	opaque := flattenOnWhite(oriented)
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, opaque, &jpeg.Options{Quality: 85}); err != nil {
		return nil, err
	}
	return filesystem.NewFileFromBytes(buf.Bytes(), filename)
}

// CompressAvatarBytes decodes an image from r, corrects EXIF orientation,
// center-crops it to a square, resizes it to fit within avatarMaxDim x
// avatarMaxDim, and re-encodes it as a JPEG file under the given filename.
// Used by both the live upload handler and sample-data seeding, so seeded
// avatars go through the exact same pipeline as a real upload.
func CompressAvatarBytes(r io.ReadSeeker, filename string) (*filesystem.File, error) {
	cfg, _, err := image.DecodeConfig(r)
	if err != nil {
		return nil, ErrAvatarInvalid
	}
	if cfg.Width*cfg.Height > avatarMaxSourcePixels {
		return nil, ErrAvatarTooLarge
	}

	if _, err := r.Seek(0, 0); err != nil {
		return nil, ErrAvatarUnreadable
	}
	orientation := readOrientation(r)

	if _, err := r.Seek(0, 0); err != nil {
		return nil, ErrAvatarUnreadable
	}
	img, _, err := image.Decode(r)
	if err != nil {
		return nil, ErrAvatarInvalid
	}

	// Shrink BEFORE the pixel-by-pixel rotate/crop passes below: resize and
	// orientation are commutative (rotate/flip only permute a square's
	// axes, and cropToSquare is center-symmetric), so downsizing first
	// bounds memory use without changing the final output.
	small := resizeToFit(img, avatarPreRotateDim, avatarPreRotateDim)
	oriented := applyOrientation(small, orientation)
	square := cropToSquare(oriented)
	resized := resizeToFit(square, avatarMaxDim, avatarMaxDim)
	opaque := flattenOnWhite(resized)

	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, opaque, &jpeg.Options{Quality: 80}); err != nil {
		slog.Error("encode avatar", "err", err)
		return nil, err
	}

	f, err := filesystem.NewFileFromBytes(buf.Bytes(), filename)
	if err != nil {
		slog.Error("build avatar file", "err", err)
		return nil, err
	}
	return f, nil
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
