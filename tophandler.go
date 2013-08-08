package main

var topHandler = NewSession(PrefixRouter(map[string]Handler{
	"*":       Static("./static"),
	"/users":  usersRouter,
	"/groups": groupsRouter,
	"/login":  &CheckMethod{"POST", &Transactional{HandlerFunc(login)}},
	"/logout": &CheckMethod{"POST", HandlerFunc(logout)},
	"/whoami": &CheckMethod{"GET", &Transactional{HandlerFunc(whoami)}},
}))
