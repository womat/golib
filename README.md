# golib
golib contains all shareable golang packages of womat.

    git clone https://github.com/womat/golib.git
    cd golib

# tag a new version

first fetch all your tags and display all of them

    git fetch --tags
    git tag -l
    ... output with list of tags ...


create a new tag for the whole library

    git tag -a v1.0.6 -m "Release 1.0.6"
    
    git push --tags

See [go module documentation](https://go.dev/doc/modules/managing-source) for more information.
