package main

var topHandler = PrefixRouter(map[string]Handler{
	"/users": usersRouter,
})
