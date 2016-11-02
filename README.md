# Slek

wip [slack](https://slack.com/) cli

<a href="https://raw.github.com/frizinak/slek/master/assets/cast.gif">
    <img src="https://raw.github.com/frizinak/slek/master/assets/cast.gif">
</a>

## Installation

`go get github.com/frizinak/slek/cmd/slek`

or [download](https://github.com/frizinak/slek/releases) a binary

should work on darwin, linux, openbsd, netbsd, freebsd and windows.

notifications:
- unixes using notify-send
- mac
- windows (10 only, timeout <16s = ~7s, >16s = ~25s)

## Example config

~/.slek  

optional: editor vim:  
- `st` terminal  
- `-c float` x11 window class so your wm can apply rules to it.  
- `-e <cmd` command your terminal should spawn  
- `nvim +'set syntax=' +'startinsert!' {}` start vim/neovim without syntax
highlighting, start in (A)ppend mode and open tempfile {}  

```
{
    "token":    "abcd-token",
    "editor":   "st -c float -e nvim +'set syntax=' +'startinsert!' {}",
    "notification_timeout": 8000
}

```

## License

GPL-3.0


## Thanks

[golang.org/x/net](https://godoc.org/golang.org/x/net)  
[0xAX/notificator](https://github.com/0xAX/notificator)  
[jroimartin/gocui](https://github.com/jroimartin/gocui)  
[mattn/go-runewidth](https://github.com/mattn/go-runewidth)  
[nlopes/slack](https://github.com/nlopes/slack)  
[nsf/termbox-go](https://github.com/nsf/termbox-go)  
[renstrom/fuzzysearch](https://github.com/renstrom/fuzzysearch)  
[mitchellh/go-wordwrap](https://github.com/mitchellh/go-wordwrap)  

suet, mokit & freddi for running an untrusted binary on their macs.
