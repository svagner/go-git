
package main

import (
"fmt"
"gopkg.in/svagner/go-git.v4"
	ssh_client "gopkg.in/svagner/go-git.v4/plumbing/client/ssh"

	"io/ioutil"
	"log"
	"os"
	"golang.org/x/crypto/ssh"
	"gopkg.in/svagner/go-git.v4/plumbing"
)

func main() {
	path:= "./test"
	os.RemoveAll(path)
	fmt.Printf("Opening repository %q ...\n", path)

	r, err := git.NewFilesystemRepository(path)
	if err != nil {
		panic(err)
	}
	sshKey, err := makeSigner("/Users/sputrya/.ssh/id_rsa")
	if err != nil {
		log.Fatalln("SSH error >>", err.Error())
	}
	err = r.Clone(&git.CloneOptions{
		URL: "git@127.0.0.1:1022/root/test.git",
		Auth: &ssh_client.PublicKeys{User: "git", Signer: sshKey},
	})
	if err != nil {
		panic(err)
	}

	iter, err := r.Commits()
	if err != nil {
		panic(err)
	}

	defer iter.Close()

	var count = 0
	err = iter.ForEach(func(commit *git.Commit) error {
		count++
		fmt.Println(commit)
		return nil
	})

	if err != nil {
		panic(err)
	}

	fmt.Println("total commits:", count)
	br, err := r.Refs()
	if err != nil {
		panic(err)
	}
	br.ForEach(func(r *plumbing.Reference) error {
		log.Println("Found branch:", r.String())
		return nil
	})

	err = r.Pull(&git.PullOptions{"origin", &ssh_client.PublicKeys{User: "git", Signer: sshKey}, "refs/heads/master", false, 0})
	if err != nil {
		panic(err)
	}
	ref, _ := r.Head()
	// ... retrieving the commit object
	commit, err := r.Commit(ref.Hash())
	if err != nil {
	panic(err)
	}
	log.Println("Got commit", ref.Hash().String())
	files,_ := commit.Files()

	// ... now we iterate the files to save to disk
	err = files.ForEach(func(f *git.File) error {
		log.Println("Commit file", f.Name)
		return nil
	})
	if err != nil {
		panic(err)
	}
}

func makeSigner(keyname string) (signer ssh.Signer, err error) {
	key, err := ioutil.ReadFile(keyname)
	if err != nil {
		return
	}
	signer, err = ssh.ParsePrivateKey([]byte(key))
	if err != nil {
		return
	}
	return
}