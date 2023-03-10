package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"html/template"
	"net/http"
	"os"
	"time"

	"github.com/cloudinary/cloudinary-go/v2"
	"github.com/cloudinary/cloudinary-go/v2/api"
	"github.com/cloudinary/cloudinary-go/v2/api/uploader"
	"github.com/golang-jwt/jwt/v5"
	"github.com/joho/godotenv"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

var tlp = template.Must(template.ParseFiles("index.html"))
var sampleSecretKey = []byte("SecretYouShouldHide")

type Post struct {
	gorm.Model
	Image   string `json:image gorm:"column:image;"`
	Name    string `json:name gorm:"column:name;"`
	Address string `json:address gorm:"column:address;"`
}
type User struct {
	gorm.Model
	Fullname string `json:"fullname"`
	Username string `json:"username" gorm:"unique"`
	Email    string `json:"email" gorm:"unique"`
	Password string `json:"password"`
}

type Result struct {
	Data     []Post
	Username string
}
type Claims struct {
	Username string `json:"username"`
	jwt.RegisteredClaims
}

func (user *User) hashPassword(password string) error {
	bytes, err := bcrypt.GenerateFromPassword([]byte(password), 14)
	if err != nil {
		return err
	}
	user.Password = string(bytes)
	return nil
}
func (user *User) CheckPassword(providedPassword string) error {
	err := bcrypt.CompareHashAndPassword([]byte(user.Password), []byte(providedPassword))
	if err != nil {
		return err
	}
	return nil
}
func uploadImage(cld *cloudinary.Cloudinary, ctx context.Context, file interface{}) string {

	resp, err := cld.Upload.Upload(ctx, file, uploader.UploadParams{
		PublicID:       "quickstart_butterfly_2",
		UniqueFilename: api.Bool(false),
		Overwrite:      api.Bool(true)})
	if err != nil {
		fmt.Println("error")
	}

	// Log the delivery URL
	return string(resp.SecureURL)
}
func authorize(r *http.Request) (string, error) {
	c, err := r.Cookie("token")
	if err != nil {
		return "", err
	}
	tknStr := c.Value
	claims := &Claims{}
	tkn, err := jwt.ParseWithClaims(tknStr, claims, func(token *jwt.Token) (interface{}, error) {
		return sampleSecretKey, nil
	})
	if err != nil {
		if err == jwt.ErrSignatureInvalid {
			return "", err
		}
		return "", err
	}
	if !tkn.Valid {
		return "", err
	}
	return claims.Username, nil
}
func listItem(db *gorm.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {

		username, _ := authorize(r)
		// q := r.URL.Query().Get("q")
		var result []Post
		// fmt.Println("q", r.URL)
		db.Model(&Post{}).Limit(100).Find(&result)
		data := Result{
			Data:     result,
			Username: username,
		}
		buf := &bytes.Buffer{}
		err := tlp.Execute(buf, data)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		buf.WriteTo(w)
	}
}
func createUser(db *gorm.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "GET" {
			t, _ := template.ParseFiles("register.html")
			t.Execute(w, nil)
		} else {
			r.ParseForm()
			user := User{
				Fullname: r.Form["fullname"][0],
				Username: r.Form["username"][0],
				Email:    r.Form["email"][0],
			}
			user.hashPassword(r.Form["password"][0])
			db.Create(&user)
			http.Redirect(w, r, "/auth/login", http.StatusFound)

		}
	}
}
func createPost(db *gorm.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		fmt.Println("method:", r.Method) //get request method
		if r.Method == "GET" {
			t, _ := template.ParseFiles("create-post.html")
			t.Execute(w, nil)
		} else {
			r.ParseMultipartForm(32 << 20)
			ctx := context.Background()
			file, _, errFile := r.FormFile("uploadfile")
			if errFile != nil {
				// Do your error handling here
				fmt.Fprintf(w, "l???i file")
			}
			defer file.Close()
			cloudinaryUrl := os.Getenv("CLOUDY")
			cld, err := cloudinary.NewFromURL(cloudinaryUrl)
			a := uploadImage(cld, ctx, file)
			if err != nil {
				fmt.Fprintf(w, "l???i")
			}
			post := Post{
				Image:   a,
				Name:    r.Form["name"][0],
				Address: r.Form["address"][0],
			}
			db.Create(&post)
			http.Redirect(w, r, "/", http.StatusFound)
		}
	}
}
func getUser(db *gorm.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var result []User
		db.Model(&User{}).Limit(100).Find(&result)
		a, _ := json.Marshal(result)
		fmt.Fprint(w, string(a))
	}
}
func login(db *gorm.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "GET" {
			t, _ := template.ParseFiles("login.html")
			t.Execute(w, nil)
		} else {
			var user User
			r.ParseForm()
			username := r.Form["username"][0]
			password := r.Form["password"][0]
			db.Where("username = ?", username).First(&user)
			err := user.CheckPassword(password)
			if err != nil {
				fmt.Fprintf(w, "error")
			} else {
				expirationTime := time.Now().Add(50 * time.Minute)
				claims := &Claims{
					Username: user.Username,
					RegisteredClaims: jwt.RegisteredClaims{
						ExpiresAt: jwt.NewNumericDate(expirationTime),
					},
				}
				token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
				tokenString, err := token.SignedString(sampleSecretKey)
				if err != nil {
					fmt.Println(err)
					fmt.Fprintf(w, "error create token")
				}

				if err != nil {
					w.WriteHeader(http.StatusInternalServerError)
					return
				}
				http.SetCookie(w, &http.Cookie{
					Name:    "token",
					Value:   tokenString,
					Expires: expirationTime,
					Path:    "/",
				})
				http.Redirect(w, r, "/", http.StatusFound)

			}
		}
	}
}
func logout(w http.ResponseWriter, r *http.Request) {
	http.SetCookie(w, &http.Cookie{
		Name:    "token",
		Expires: time.Now(),
		Path:    "/",
	})
	http.Redirect(w, r, "/", http.StatusFound)
}
func main() {
	db, err := gorm.Open(sqlite.Open("test.db"), &gorm.Config{})
	if err != nil {
		panic("failed to connect database")
	}
	godotenv.Load()
	port := os.Getenv("PORT")

	db.AutoMigrate(&Post{})
	db.AutoMigrate(&User{})
	fs := http.FileServer(http.Dir("assets"))
	mux := http.NewServeMux()
	mux.Handle("/assets/", http.StripPrefix("/assets/", fs))
	mux.HandleFunc("/create-post", createPost(db))
	mux.HandleFunc("/", listItem(db))
	mux.HandleFunc("/auth/register", createUser(db))
	mux.HandleFunc("/auth/login", login(db))
	mux.HandleFunc("/auth/logout", logout)
	mux.HandleFunc("/users", getUser(db))
	http.ListenAndServe(":"+port, mux)
}
