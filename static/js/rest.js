function rest(method, url, callback, data) {
	var xhr = new XMLHttpRequest(), response;
	xhr.timeout = 5000;
	xhr.open(method, url);
	xhr.setRequestHeader('Content-Type', 'application/json; charset=utf-8');
	xhr.onreadystatechange = function () {
		if (xhr.readyState != XMLHttpRequest.DONE) {
			return;		
		}
		try {
			response = JSON.parse(xhr.responseText);
		} catch (e) {
		}
		if (typeof callback == "function") {
			callback(xhr.status, response);
		} else {
			console.log(xhr.status, response);
		}
	};
	if (typeof data != "undefined") {
		xhr.send(JSON.stringify(data));
	} else {
		xhr.send();
	}

}

rest('POST', '/login', null, {email:"korandi.z@gmail.com", password:"password"});
