package main

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"golang.org/x/net/html"
)

func download_to(u *url.URL, outbase, fpath string) error {
	fmt.Print(u, "->", fpath, " ... ")

	fh, err := os.CreateTemp(outbase, "download_tmp_")
	if err != nil {
		return err
	}
	defer func() {
		_ = os.Remove(fh.Name())
	}()
	defer fh.Close()

	resp, err := http.Get(u.String())
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return errors.New(resp.Status)
	}

	n, err := io.Copy(fh, resp.Body)
	if err != nil {
		return err
	}

	if err := fh.Close(); err != nil {
		return err
	}

	fmt.Println(resp.Status)

	if n != resp.ContentLength && resp.ContentLength != -1 {
		return fmt.Errorf("Content length mismatch (%d != %d)", n, resp.ContentLength)
	}

	if err := os.Rename(fh.Name(), fpath); err != nil {
		return err
	}

	return nil
}

func download(u *url.URL, outbase string) (string, error) {
	dir := outbase + "extern/" + u.Host

	newpath := filepath.Clean(u.Path)

	// Only a token effort at sanitization--- Don't run this against untrusted content
	if strings.Contains(newpath, "..") {
		return "", fmt.Errorf("Invalid download URL %#v", u)
	}

	new_full := dir + newpath
	relative_path := "/" + strings.TrimPrefix(new_full, outbase)

	if _, err := os.Stat(new_full); err == nil {
		// Already downloaded
		return relative_path, nil
	}

	if err := os.MkdirAll(filepath.Dir(new_full), 0755); err != nil {
		return "", err
	}

	if err := download_to(u, outbase, new_full); err != nil {
		return "", err
	}

	time.Sleep(time.Second)

	return relative_path, nil
}

func walk_doc(node *html.Node, outbase, out_dir string) error {
	c := node.FirstChild

	for c != nil {
		if c.Type != html.ElementNode {
			c = c.NextSibling
			continue
		}

		for i, attr := range c.Attr {
			key := strings.ToLower(attr.Key)

			if key == "src" || key == "href" {
				u, err := url.Parse(attr.Val)
				if err != nil {
					return err
				}

				p := strings.ToLower(u.Path)
				match := false
				for _, suf := range []string{".jpeg", ".jpg", ".png", ".tif", ".tiff", ".hiec", ".mp4", ".avif", ".mpeg", ".mpg", ".mp3", ".mp4", ".gif"} {
					if strings.HasSuffix(p, suf) {
						match = true
					}
				}
				if match {
					fixed_val, err := download(u, outbase)
					if err != nil {
						//return fmt.Errorf("Cannot download %s: %w", u, err)
						fmt.Println(err)
					} else {
						c.Attr[i].Val = fixed_val
					}
				}
			}
		}

		// Recurse down
		if err := walk_doc(c, outbase, out_dir); err != nil {
			return err
		}

		c = c.NextSibling
		continue
	}

	return nil

}

func walk_file(fpath, outbase, out_dir, out_fpath string) error {
	if err := os.MkdirAll(out_dir, 0755); err != nil {
		return err
	}

	fh, err := os.Open(fpath)
	if err != nil {
		return err
	}
	defer fh.Close()

	doc, err := html.Parse(fh)
	if err != nil {
		return err
	}

	if err := walk_doc(doc, outbase, out_dir); err != nil {
		return err
	}

	outfh, err := os.Create(out_fpath)
	if err != nil {
		return err
	}
	defer outfh.Close()

	if err := html.Render(outfh, doc); err != nil {
		return err
	}

	if err := outfh.Close(); err != nil {
		return err
	}

	fmt.Println(fpath, "->", out_fpath)
	return nil
}

func walk_fpath(inbase, outbase string) fs.WalkDirFunc {
	return func(s string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() {
			if !strings.HasSuffix(s, ".html") {
				return nil
			}
			base, err := filepath.Abs(inbase)
			if err != nil {
				return err
			}
			base += "/"
			sabs, err := filepath.Abs(s)
			if err != nil {
				return err
			}
			out_dir := outbase + strings.TrimPrefix(filepath.Dir(sabs)+"/", base)
			out_fpath := out_dir + filepath.Base(s)

			if err := walk_file(s, outbase, filepath.Clean(out_dir), out_fpath); err != nil {
				return fmt.Errorf("Cannot walk file %s: %w", s, err)
			}
		}
		return nil
	}
}

func main() {
	fpaths := os.Args[1:]
	if len(fpaths) == 0 {
		fpaths = []string{"."}
	}
	for _, fpath := range fpaths {
		if err := filepath.WalkDir(fpath, walk_fpath(fpath, "out/")); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
	}
}
