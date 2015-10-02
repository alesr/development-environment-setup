package main

import (
	"bytes"
	"fmt"
	"github.com/alesr/error-util"
	"github.com/alesr/file-util"
	"golang.org/x/crypto/ssh"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// A project is made of project fields which has a program on it.
type program struct {
	setup              []string
	postUpdateFilename string
}

type projectField struct {
	name, label, inputQuestion, errorMsg, validationMsg string
	program                                             program
}

type project struct {
	projectname, hostname, pwd, port, typ projectField
}

func main() {

	// Initialization
	project := new(project)

	// Let's build our project!
	project.assemblyLine()

	project.connect()
	fmt.Println("Environment configuration done.")
}

func (p *project) assemblyLine() {
	// project name
	p.projectname.inputQuestion = "project name: "
	p.projectname.label = "projectname"
	p.projectname.errorMsg = "error getting the project's name: "
	p.projectname.validationMsg = "make sure you type a valid name for your project (3 to 20 characters)."
	p.projectname.name = checkInput(ask4Input(&p.projectname))

	// Hostname
	p.hostname.inputQuestion = "hostname: "
	p.hostname.label = "hostname"
	p.hostname.errorMsg = "error getting the project's hostname: "
	p.hostname.validationMsg = "make sure you type a valid hostname for your project. it must contain '.com', '.pt' or '.org', for example.)."
	p.hostname.name = checkInput(ask4Input(&p.hostname))

	// Password
	p.pwd.inputQuestion = "password: "
	p.pwd.label = "pwd"
	p.pwd.errorMsg = "error getting the project's password: "
	p.pwd.validationMsg = "type a valid password. It must contain at least 6 digits"
	p.pwd.name = checkInput(ask4Input(&p.pwd))

	// Port
	p.port.inputQuestion = "port (default 22): "
	p.port.label = "port"
	p.port.errorMsg = "error getting the project's port"
	p.port.validationMsg = "only digits allowed. min 0, max 9999."
	p.port.name = checkInput(ask4Input(&p.port))

	// Type
	p.typ.inputQuestion = "[1] Yii\n[2] WP or goHugo\nEnter project type: "
	p.typ.label = "type"
	p.typ.errorMsg = "error getting the project's type"
	p.typ.validationMsg = "pay attention to the options"
	p.typ.name = checkInput(ask4Input(&p.typ))

	// Now we need to know which instalation we going to make.
	// And once we get to know it, let's load the setup with
	// the aproppriate set of files and commands.
	if p.typ.name == "Yii" {

		// Loading common steps into the selected setup
		p.typ.program.setup = []string{}
		p.typ.program.postUpdateFilename = "post-update-yii"
	} else {

		// Loading common steps into the selected setup
		p.typ.program.setup = []string{
			"echo -e '[User]\nname = Pipi, server girl' > .gitconfig",
			"cd ~/www/www/ && git init",
			"cd ~/www/www/ && touch readme.txt && git add . ",
			"cd ~/www/www/ && git commit -m 'on the beginning was the commit'",
			"cd ~/private/ && mkdir repos && cd repos && mkdir " + p.projectname.name + "_hub.git && cd " + p.projectname.name + "_hub.git && git --bare init",
			"cd ~/www/www && git remote add hub ~/private/repos/" + p.projectname.name + "_hub.git && git push hub master",
			"post-update configuration",
			"cd ~/www/www && git remote add hub ~/private/repos/" + p.projectname.name + "_hub.git/hooks && chmod 755 post-update",
			p.projectname.name + ".dev",
			"git clone",
		}
		p.typ.program.postUpdateFilename = "post-update-wp"
	}
}

// Takes the assemblyLine's data and mount the prompt for the user.
func ask4Input(field *projectField) (*projectField, string) {

	fmt.Print(field.inputQuestion)

	var input string
	_, err := fmt.Scanln(&input)

	// The port admits empty string as user input. Setting the default value to "22".
	if err != nil && err.Error() == "unexpected newline" && field.label != "port" {
		ask4Input(field)
	} else if err != nil && err.Error() == "unexpected newline" {
		input = "22"
		checkInput(field, input)
	} else if err != nil {
		log.Fatal(field.errorMsg, err)
	}
	return field, input
}

// Check invalid parameters on the user input.
func checkInput(field *projectField, input string) string {

	switch inputLength := len(input); field.label {
	case "projectname":
		if inputLength > 20 {
			fmt.Println(field.validationMsg)
			ask4Input(field)
		}
	case "hostname":
		if inputLength <= 5 {
			fmt.Println(field.validationMsg)
			ask4Input(field)
		}
	case "pwd":
		if inputLength <= 6 {
			fmt.Println(field.validationMsg)
			ask4Input(field)
		}
	case "port":
		if inputLength == 0 {
			input = "22"
		} else if inputLength > 4 {
			fmt.Println(field.validationMsg)
			ask4Input(field)
		}
	case "type":
		if input != "1" && input != "2" {
			fmt.Println(field.validationMsg)
			ask4Input(field)
		} else if input == "1" {
			input = "Yii"
		} else if input == "2" {
			input = "WP"
		}
	}

	// Everything looks fine so lets set the value.
	return input
}

// Creates a ssh connection between the local machine and the remote server.
func (p *project) connect() {

	// config *ssh.ClientConfig

	// SSH connection config
	config := &ssh.ClientConfig{
		User: p.projectname.name,
		Auth: []ssh.AuthMethod{
			ssh.Password(p.pwd.name),
		},
	}

	fmt.Println("Trying connection...")

	conn, err := ssh.Dial("tcp", fmt.Sprintf("%s:%s", p.hostname.name, p.port.name), config)
	errorUtil.CheckError("Failed to dial: ", err)
	fmt.Println("Connection established.")

	session, err := conn.NewSession()
	errorUtil.CheckError("Failed to build session: ", err)
	defer session.Close()

	// Loops over the slice of commands to be executed on the remote.
	for step := range p.typ.program.setup {

		switch p.typ.program.setup[step] {
		case "post-update configuration":
			p.secureCopy(conn)
		case p.projectname.name + ".dev":
			p.makeDirOnLocal(step)
		case "git clone":
			p.gitOnLocal(step)
		default:
			p.installOnRemote(step, conn)
		}
	}
}

func (p *project) installOnRemote(step int, conn *ssh.Client) {

	// Git and some other programs can send us an unsuccessful exit (< 0)
	// even if the command was successfully executed on the remote shell.
	// On these cases, we want to ignore those errors and move onto the next step.
	ignoredError := "Reason was:  ()"

	// Creates a session over the ssh connection to execute the commands
	session, err := conn.NewSession()
	errorUtil.CheckError("Failed to build session: ", err)
	defer session.Close()

	var stdoutBuf bytes.Buffer
	session.Stdout = &stdoutBuf

	fmt.Println(p.typ.program.setup[step])

	err = session.Run(p.typ.program.setup[step])

	if err != nil && !strings.Contains(err.Error(), ignoredError) {
		log.Printf("Command '%s' failed on execution", p.typ.program.setup[step])
		log.Fatal("Error on command execution: ", err.Error())
	}
}

// Secure Copy a file from local machine to remote host.
func (p *project) secureCopy(conn *ssh.Client) {
	session, err := conn.NewSession()
	errorUtil.CheckError("Failed to build session: ", err)
	defer session.Close()

	var stdoutBuf bytes.Buffer
	session.Stdout = &stdoutBuf

	go func() {
		w, _ := session.StdinPipe()
		defer w.Close()
		content := fileUtil.ReadFile("post-update-files" + string(filepath.Separator) + p.typ.program.postUpdateFilename)
		fmt.Fprintln(w, "C0644", len(content), "post-update")
		fmt.Fprint(w, content)
		fmt.Fprint(w, "\x00")
	}()

	if err := session.Run("scp -qrt ~/private/repos/" + p.projectname.name + "_hub.git/hooks"); err != nil {
		log.Fatal("Failed to run SCP: " + err.Error())
	}
}

// Creates a directory on the local machine. Case the directory already exists
// remove the old one and runs the function again.
func (p *project) makeDirOnLocal(step int) {

	fmt.Println("Creating directory...")

	// Get the user home directory path.
	homeDir := fileUtil.FindUserHomeDir()

	// The dir we want to create.
	dir := homeDir + string(filepath.Separator) + "sites" + string(filepath.Separator) + p.typ.program.setup[step]

	// Check if the directory already exists.
	if _, err := os.Stat(dir); os.IsNotExist(err) {
		err := os.Mkdir(dir, 0755)
		errorUtil.CheckError("Failed to create directory.", err)
		fmt.Println(dir + " successfully created.")
	} else {
		fmt.Println(dir + " already exist.\nRemoving old and creating new...")

		// Remove the old one.
		if err := os.RemoveAll(dir); err != nil {
			log.Fatalf("Error removing %s\n%s", dir, err)
		}
		p.makeDirOnLocal(step)
	}
}

func (p *project) gitOnLocal(step int) {
	homeDir := fileUtil.FindUserHomeDir()

	if err := os.Chdir(homeDir + string(filepath.Separator) + "sites" + string(filepath.Separator) + p.projectname.name + ".dev/"); err != nil {
		log.Fatal("Failed to change directory.")
	}

	repo := "ssh://" + p.projectname.name + "@" + p.hostname.name + "/home/" + p.projectname.name + "/private/repos/" + p.projectname.name + "_hub.git"

	cmd := exec.Command("git", "clone", repo, ".")
	if err := cmd.Run(); err != nil {
		log.Fatal("Failed to execute git clone: ", err)
	}

}
