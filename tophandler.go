package main

var topHandler = PrefixRouter(map[string]Handler{
	"*":       Static("./static"),
	"/users":  usersRouter,
	"/groups": groupsRouter,
})
