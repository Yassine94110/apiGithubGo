package main

import (
	"archive/zip"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"sync"
	"time"

	"github.com/joho/godotenv"
)

type Repository struct {
	Name         string    `json:"name"`
	LastModified time.Time `json:"pushed_at"`
}

func main() {

	err := godotenv.Load()
	if err != nil {
		fmt.Println("Erreur lors du chargement du fichier .env:", err)
		return
	}

	username := os.Getenv("GITHUB_USERNAME")
	token := os.Getenv("GITHUB_TOKEN")

	err = getAndPrintRecentRepositories(username, token)
	if err != nil {
		fmt.Println("Erreur:", err)
	}
}

func getAndPrintRecentRepositories(username, token string) error {
	repos, err := getRepositories(username, token)
	if err != nil {
		return err
	}

	sort.Slice(repos, func(i, j int) bool {
		return repos[i].LastModified.After(repos[j].LastModified)
	})

	if len(repos) > 100 {
		repos = repos[:100]
	}

	err = createCSV(username, repos)
	if err != nil {
		return err
	}

	var wg sync.WaitGroup

	for _, repo := range repos {
		wg.Add(1)                  // Ajouter un compteur pour chaque dépôt
		go func(repo Repository) { // Lancer une goroutine pour traiter le dépôt
			defer wg.Done() // Décrémenter le compteur à la fin

			cloneDir := fmt.Sprintf("clones/%s", repo.Name)
			cloneURL := fmt.Sprintf("https://github.com/%s/%s.git", username, repo.Name)
			err := exec.Command("git", "clone", cloneURL, cloneDir).Run()
			if err != nil {
				fmt.Println("Erreur:", err)
				return
			}

			err = exec.Command("git", "-C", cloneDir, "pull").Run()
			if err != nil {
				fmt.Printf("Erreur lors du git pull%s: %v\n", repo.Name, err)
				return
			}

			err = exec.Command("git", "-C", cloneDir, "fetch").Run()
			if err != nil {
				fmt.Printf("Erreur lors du git fetch %s: %v\n", repo.Name, err)
				return
			}

			err = createZipArchive(cloneDir, fmt.Sprintf("archives/%s.zip", repo.Name))
			if err != nil {
				fmt.Printf("Erreur lors dde la création du ZIP %s: %v\n", repo.Name, err)
			}
		}(repo)
	}

	wg.Wait() // Attendre que toutes les goroutines soient terminées

	for i, repo := range repos {
		fmt.Printf("%d. Nom du référentiel: %s\n", i+1, repo.Name)
		fmt.Printf("   Date de dernière modification: %s\n", repo.LastModified)
	}

	return nil
}

func getRepositories(username, token string) ([]Repository, error) {
	url := fmt.Sprintf("https://api.github.com/users/%s/repos", username)
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}

	req.Header.Set("Authorization", "token "+token)

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var repos []Repository
	decoder := json.NewDecoder(resp.Body)
	err = decoder.Decode(&repos)
	if err != nil {
		return nil, err
	}

	return repos, nil
}

func createCSV(username string, repos []Repository) error {
	currentDate := time.Now().Format("2006-01-02")

	fileName := fmt.Sprintf("csv/%s_%s.csv", username, currentDate)
	file, err := os.Create(fileName)
	if err != nil {
		return err
	}
	defer file.Close()

	csvWriter := csv.NewWriter(file)

	headers := []string{"Username", "Date de récupération"}
	csvWriter.Write(headers)

	data := []string{username, currentDate}
	csvWriter.Write(data)

	for _, repo := range repos {
		data = []string{repo.Name, repo.LastModified.String()}
		csvWriter.Write(data)
	}

	csvWriter.Flush()

	return csvWriter.Error()
}

func createZipArchive(sourceDir, targetFile string) error {
	zipFile, err := os.Create(targetFile)
	if err != nil {
		return err
	}
	defer zipFile.Close()

	zipWriter := zip.NewWriter(zipFile)
	defer zipWriter.Close()

	err = filepath.Walk(sourceDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		relativePath, err := filepath.Rel(sourceDir, path)
		if err != nil {
			return err
		}

		if info.IsDir() {
			return nil
		}

		sourceFile, err := os.Open(path)
		if err != nil {
			return err
		}
		defer sourceFile.Close()

		zipEntry, err := zipWriter.Create(relativePath)
		if err != nil {
			return err
		}

		_, err = io.Copy(zipEntry, sourceFile)
		if err != nil {
			return err
		}
		return nil
	})
	return err
}
