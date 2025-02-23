# golib
golib contains all shareable golang packages of womat.



    git clone https://github.com/womat/golib.git
    cd golib

    go mod init github.com/womat/golib



# golib

first fetch all your tags and display all of them

    git fetch --tags
    git tag -l
    ... output with list of tags ...

you have to prefix the tag with the folder name, e.g.: commandLine/v1.0.0

    git tag -a crypt/v1.0.2 -m "Release 1.0.2"
    git tag -a jwt_util/v1.0.2 -m "Release 1.0.2"
    git tag -a crypt/v1.0.2 -m "Release 1.0.2"
    git tag -a keyvalue/v1.0.2 -m "Release 1.0.2"
    git tag -a rpi/v1.0.3 -m "Release 1.0.3"
    git tag -a web/v1.0.3 -m "Release 1.0.3"
    git tag -a xlog/v1.0.2 -m "Release 1.0.2"
    git tag -a manchester/v1.0.1 -m "Release 1.0.1"

after you create a new tag for a specific package you also have to create a new tag for the whole library

    git tag -a v1.0.4 -m "Release 1.0.4"
    
    git push --tags

See [go module documentation](https://go.dev/doc/modules/managing-source) for more information.
