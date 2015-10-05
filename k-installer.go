package main

import (
	"bytes"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"

	"github.com/alesr/error-util"
	"github.com/alesr/file-util"
	"golang.org/x/crypto/ssh"
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

// Project - Defines a K project.
type Project struct {
	projectname, host, pwd, port, typ, sshkey projectField
}

var sep = string(filepath.Separator)

func main() {

	// Initialization
	project := new(Project)

	project.assemblyLine()

	project.connect()

	fmt.Println("Environment configuration done.\nHit enter to exit O.o")
	var mode string
	_, err := fmt.Scanln(&mode)
	os.Exit(0)
	errorUtil.CheckError("Failed to get user input: ", err)
}

func (p *Project) assemblyLine() {
	// project name
	p.projectname.inputQuestion = "project name: "
	p.projectname.label = "projectname"
	p.projectname.errorMsg = "error getting the project's name: "
	p.projectname.validationMsg = "make sure you type a valid name for your project (3 to 20 characters)."
	p.projectname.name = checkInput(ask4Input(&p.projectname))

	// Hostname
	p.host.inputQuestion = "hostname: "
	p.host.label = "hostname"
	p.host.errorMsg = "error getting the project's hostname: "
	p.host.validationMsg = "make sure you type a valid hostname for your project. it must contain '.com', '.pt' or '.org', for example.)."
	p.host.name = checkInput(ask4Input(&p.host))

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

	p.sshkey.inputQuestion = "Public ssh key name: "
	p.sshkey.label = "sshkey"
	p.sshkey.errorMsg = "error getting the key name"
	p.sshkey.validationMsg = "pay attention to the options"
	p.sshkey.name = checkInput(ask4Input(&p.sshkey))

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
			"git clone on " + p.projectname.name + ".dev",
			"copying ssh public key",
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
func (p *Project) connect() {

	// SSH connection config
	config := &ssh.ClientConfig{
		User: p.projectname.name,
		Auth: []ssh.AuthMethod{
			ssh.Password(p.pwd.name),
		},
	}

	fmt.Println("\nTrying connection...")

	conn, err := ssh.Dial("tcp", fmt.Sprintf("%s:%s", p.host.name, p.port.name), config)
	errorUtil.CheckError("Failed to dial: ", err)
	fmt.Println("Connection established.")

	session, err := conn.NewSession()
	errorUtil.CheckError("Failed to build session: ", err)
	defer session.Close()

	// Loops over the slice of commands to be executed on the remote.
	for step := range p.typ.program.setup {

		switch p.typ.program.setup[step] {
		case "post-update configuration":
			filepath := "post-update-files" + sep + p.typ.program.postUpdateFilename
			p.secureCopy(conn, "post-update configuration", filepath)
		case p.projectname.name + ".dev":
			p.makeDirOnLocal(step)
		case "git clone on " + p.projectname.name + ".dev":
			p.gitOnLocal(step)
		case "copying ssh public key":
			filepath := fileUtil.FindUserHomeDir() + sep + ".ssh/" + p.sshkey.name + ".pub"
			p.secureCopy(conn, "copying ssh public key", filepath)
		default:
			p.installOnRemote(step, conn)
		}
	}
}

func (p *Project) installOnRemote(step int, conn *ssh.Client) {

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
func (p *Project) secureCopy(conn *ssh.Client, phase, filepath string) {
	session, err := conn.NewSession()
	errorUtil.CheckError("Failed to build session: ", err)
	defer session.Close()

	var stdoutBuf bytes.Buffer
	session.Stdout = &stdoutBuf

	var file, dest string
	if phase == "post-update configuration" {
		file = "post-update"
		dest = "scp -qrt ~/private/repos/" + p.projectname.name + "_hub.git/hooks"
	} else {
		file = "authorized_keys"
		dest = "scp -qrt ~" + sep + ".ssh"
	}

	go func() {
		w, _ := session.StdinPipe()
		defer w.Close()
		content := fileUtil.ReadFile(filepath)
		fmt.Fprintln(w, "C0644", len(content), file)
		fmt.Fprint(w, content)
		fmt.Fprint(w, "\x00")
	}()

	fmt.Printf("%s... %s\n", file, dest)

	ignoredError := "Reason was:  ()"
	if err := session.Run(dest); err != nil && !strings.Contains(err.Error(), ignoredError) {
		log.Fatal("Failed to run SCP: " + err.Error())
	}
}

// Creates a directory on the local machine. Case the directory already exists
// remove the old one and runs the function again.
func (p *Project) makeDirOnLocal(step int) {

	fmt.Println("Creating directory...")

	// Get the user home directory path.
	homeDir := fileUtil.FindUserHomeDir()

	// The dir we want to create.
	dir := homeDir + sep + "sites" + sep + p.typ.program.setup[step]

	// Check if the directory already exists.
	if _, err := os.Stat(dir); os.IsNotExist(err) {
		err := os.Mkdir(dir, 0755)
		errorUtil.CheckError("Failed to create directory.", err)
		fmt.Println(dir + " successfully created.")
	} else {
		fmt.Println(dir + " already exist.\nRemoving old and creating new...")

		if runtime.GOOS == "windows" {
			fmt.Println("Hello from Windows")
		}

		// Remove the old one.
		if err := os.RemoveAll(dir); err != nil {
			log.Fatalf("Error removing %s\n%s", dir, err)
		}
		p.makeDirOnLocal(step)
	}
}

// Git clone on local machine
func (p *Project) gitOnLocal(step int) {

	homeDir := fileUtil.FindUserHomeDir()

	if err := os.Chdir(homeDir + sep + "sites" + sep + p.projectname.name + ".dev" + sep); err != nil {
		log.Fatal("Failed to change directory.")
	} else {
		repo := "ssh://" + p.projectname.name + "@" + p.host.name + "/home/" + p.projectname.name + "/private/repos/" + p.projectname.name + "_hub.git"

		fmt.Println("Cloning repository...")

		cmd := exec.Command("git", "clone", repo, ".")

		// Stdout buffer
		cmdOutput := &bytes.Buffer{}
		// Attach buffer to command
		cmd.Stdout = cmdOutput

		var waitStatus syscall.WaitStatus

		if err := cmd.Run(); err != nil {
			log.Println("Failed to execute git clone: ", err)
			// Did the command fail because of an unsuccessful exit code
			if exitError, ok := err.(*exec.ExitError); ok {
				waitStatus = exitError.Sys().(syscall.WaitStatus)
				printLocalCmdOutput([]byte(fmt.Sprintf("%d", waitStatus.ExitStatus())))
			}
		}
	}
}

func printLocalCmdOutput(out []byte) {
	if len(out) > 0 {
		fmt.Printf("==> Output: %s\n", out)
	}
}
