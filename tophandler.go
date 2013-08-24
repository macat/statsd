package main

var topHandler = NewSession(PrefixRouter(map[string]Handler{
	"*":            Static("./web-client/tmp"),
	"/users":       usersRouter,
	"/groups":      groupsRouter,
	"/login":       &CheckMethod{"POST", &Transactional{HandlerFunc(login)}},
	"/logout":      &CheckMethod{"POST", HandlerFunc(logout)},
	"/whoami":      &CheckMethod{"GET", &Transactional{HandlerFunc(whoami)}},
	"/permissions": permissionsRouter,
	"/dashboards":  dashboardsRouter,
}))
