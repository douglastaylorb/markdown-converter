package main

import (
	"archive/zip"
	"fmt"
	"io"
	"log"
	"mime/multipart"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
)

func main() {
	if err := checkPandoc(); err != nil {
		log.Fatalf("Erro crítico: %v", err)
	}
	e := echo.New()

	// Configurar CORS
	e.Use(middleware.CORSWithConfig(middleware.CORSConfig{
		AllowOrigins: []string{"*"},
		AllowMethods: []string{http.MethodGet, http.MethodPost},
	}))

	e.POST("/convert", handleConvert)

	e.Logger.Fatal(e.Start(":8080"))
}

func handleConvert(c echo.Context) error {
	log.Println("Iniciando processo de conversão")

	// Obter o arquivo do formulário
	file, err := c.FormFile("file")
	if err != nil {
		log.Printf("Erro ao obter arquivo: %v", err)
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "No file uploaded"})
	}
	log.Printf("Arquivo recebido: %s", file.Filename)

	// Criar diretório de uploads se não existir
	uploadsDir := "uploads"
	if err := os.MkdirAll(uploadsDir, 0755); err != nil {
		log.Printf("Erro ao criar diretório de uploads: %v", err)
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Failed to create uploads directory"})
	}

	// Salvar o arquivo zip
	zipPath := filepath.Join(uploadsDir, file.Filename)
	if err := saveUploadedFile(file, zipPath); err != nil {
		log.Printf("Erro ao salvar arquivo: %v", err)
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Failed to save file"})
	}

	// Extrair o zip
	extractPath := filepath.Join(uploadsDir, "extracted_"+filepath.Base(zipPath))
	if err := unzipFile(zipPath, extractPath); err != nil {
		log.Printf("Erro ao extrair zip: %v", err)
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Failed to extract zip: " + err.Error()})
	}

	// Encontrar o arquivo markdown
	mdFile, err := findMarkdownFile(extractPath)
	if err != nil {
		log.Printf("Erro ao encontrar arquivo markdown: %v", err)
		return c.JSON(http.StatusBadRequest, map[string]string{"error": err.Error()})
	}

	// Converter para DOCX
	docxPath := filepath.Join(extractPath, "output.docx")
	if err := convertToDOCX(mdFile, docxPath); err != nil {
		log.Printf("Erro na conversão: %v", err)
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Conversion failed: " + err.Error()})
	}

	// Configurar a limpeza para ser executada após o envio do arquivo
	defer func() {
		if err := os.RemoveAll(extractPath); err != nil {
			log.Printf("Erro ao remover diretório temporário: %v", err)
		}
		if err := os.Remove(zipPath); err != nil {
			log.Printf("Erro ao remover arquivo zip: %v", err)
		}
	}()

	log.Println("Conversão concluída com sucesso")

	// Enviar o arquivo convertido
	return c.Attachment(docxPath, "converted.docx")
}

func saveUploadedFile(file *multipart.FileHeader, dst string) error {
	src, err := file.Open()
	if err != nil {
		return err
	}
	defer src.Close()

	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()

	_, err = io.Copy(out, src)
	return err
}

func findMarkdownFile(dir string) (string, error) {
	var mdFile string
	err := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if filepath.Ext(path) == ".md" {
			mdFile = path
			return io.EOF // para parar a busca
		}
		return nil
	})

	if err != nil && err != io.EOF {
		return "", fmt.Errorf("error walking the path %s: %v", dir, err)
	}

	if mdFile == "" {
		return "", fmt.Errorf("no markdown file found in zip")
	}

	return mdFile, nil
}

func convertToDOCX(mdFile, docxPath string) error {
	cmd := exec.Command("pandoc", "-f", "markdown", "-t", "docx", mdFile, "-o", docxPath, "--extract-media=.")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("pandoc error: %v, output: %s", err, string(output))
	}
	return nil
}
func unzipFile(src, dest string) error {
	log.Printf("Iniciando extração do arquivo: %s para %s", src, dest)

	r, err := zip.OpenReader(src)
	if err != nil {
		log.Printf("Erro ao abrir o arquivo zip: %v", err)
		return err
	}
	defer r.Close()

	if err := os.MkdirAll(dest, 0755); err != nil {
		log.Printf("Erro ao criar o diretório de destino: %v", err)
		return err
	}

	for _, f := range r.File {
		log.Printf("Extraindo: %s", f.Name)

		// Garantir que o caminho de destino esteja dentro do diretório de destino
		filePath := filepath.Join(dest, f.Name)
		if !strings.HasPrefix(filePath, filepath.Clean(dest)+string(os.PathSeparator)) {
			return fmt.Errorf("arquivo inválido detectado: %s", f.Name)
		}

		if f.FileInfo().IsDir() {
			log.Printf("Criando diretório: %s", filePath)
			os.MkdirAll(filePath, os.ModePerm)
			continue
		}

		if err := os.MkdirAll(filepath.Dir(filePath), os.ModePerm); err != nil {
			log.Printf("Erro ao criar diretório para arquivo: %v", err)
			return err
		}

		dstFile, err := os.OpenFile(filePath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, f.Mode())
		if err != nil {
			log.Printf("Erro ao criar arquivo: %v", err)
			return err
		}

		srcFile, err := f.Open()
		if err != nil {
			log.Printf("Erro ao abrir arquivo dentro do zip: %v", err)
			dstFile.Close()
			return err
		}

		_, err = io.Copy(dstFile, srcFile)
		srcFile.Close()
		dstFile.Close()

		if err != nil {
			log.Printf("Erro ao copiar conteúdo do arquivo: %v", err)
			return err
		}
	}

	log.Printf("Extração concluída com sucesso")
	return nil
}

func checkPandoc() error {
	cmd := exec.Command("pandoc", "--version")
	output, err := cmd.CombinedOutput()
	if err != nil {
		log.Printf("Erro ao verificar versão do Pandoc: %v", err)
		return fmt.Errorf("Pandoc não está instalado ou não é executável: %w", err)
	}
	log.Printf("Versão do Pandoc: %s", string(output))
	return nil
}
