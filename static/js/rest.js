function rest(method, url, callback, data) {
	var xhr = new XMLHttpRequest(), data;
	xhr.timeout = 5000;
	xhr.open(method, url);
	xhr.setRequestHeader('Content-Type', 'application/json; charset=utf-8');
	xhr.onreadystatechange = function () {
		if (xhr.readyState != XMLHttpRequest.DONE) {
			return;		
		}
		try {
			data = JSON.parse(xhr.responseText);
		} catch (e) {
		}
		if (typeof callback == "function") {
			callback(xhr.status, data);
		} else {
			console.log(xhr.status, data);
		}
	};
	if (typeof data != "undefined") {
		xhr.send(JSON.stringify(data));
	} else {
		xhr.send();
	}

}
