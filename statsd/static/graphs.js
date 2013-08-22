window.onload = function () {
	W1 = new Widget("w1", "kozo_rx_bytes", "kozo_tx_bytes", false);
	W2 = new Widget("w2", "kozo_rx_bytes", "kozo_tx_bytes", true);
	W3 = new Widget("w3", "kozo_rx_packets", "kozo_tx_packets", false);
	W4 = new Widget("w4", "kozo_rx_packets", "kozo_tx_packets", true);
	W5 = new Widget("w5", "macat_rx_bytes", "macat_tx_bytes", false);
	W6 = new Widget("w6", "macat_rx_bytes", "macat_tx_bytes", true);
	W7 = new Widget("w7", "macat_rx_packets", "macat_tx_packets", false);
	W8 = new Widget("w8", "macat_rx_packets", "macat_tx_packets", true);
};

function Widget(id, rx, tx, minutes) {
	this.rx = rx;
	this.tx = tx;
	this.el = window.document.getElementById(id);
	this.minutes = !!minutes;
	this.ts = [null, null];
	this.data = [null, null];
	this.xhr = [null, null];
	this.ws = [null, null];
	this.on = false;
	this.loading = false;
	this.blink = null;
	var widget = this;
	this.el.ondblclick = function (ev) {
		if (widget.on) {
			widget.stop();
		} else {
			widget.start();
		}
	};
}

Widget.prototype.start = function () {
	if (this.on || this.loading) return;
	this.loading = true;
	var widget = this;
	this.blink = setInterval(function () { widget.toggleLed(); }, 100);
	this.loadData(0, this.rx);
	this.loadData(1, this.tx);
};

Widget.prototype.stop = function () {
	if (!this.loading && !this.on) return;
	this.ts = [null, null];
	this.data = [null, null];
	this.xhr[0].abort();
	this.xhr[1].abort();
	this.xhr = [null, null];
	this.ws[0].close();
	this.ws[1].close();
	this.ws = [null, null];
	this.on = false;
	this.loading = false;
	clearInterval(this.blink)
	this.ledOff();
	var canvas = this.el.getElementsByTagName('canvas')[0];
	canvas.getContext('2d').clearRect(0, 0, canvas.width, canvas.height);
};

Widget.prototype.loadData = function (ch, name) {
	var live, params
	if (this.minutes) {
		live = "/";
		var now = ((new Date).getTime()/1000)|0;
		var tz = -60*(new Date).getTimezoneOffset();
		now -= now % 60;
		params = "?gran=1&from="+(now-300*60)+"&offs="+tz+"&length=600";
	} else {
		live = "/live:";
		params = "";
	}
	var ws = new WebSocket("ws://"+window.location.host+live+name+":meter"+params);
	var xhr = new XMLHttpRequest()
	this.xhr[ch] = xhr;
	this.ws[ch] = ws;
	this.data[ch] = [];
	var widget = this;
	ws.onmessage = function (ev) {
		if (widget.ws[ch] !== ws) return;
		var d = JSON.parse(ev.data);
		if (widget.ts[ch] === null) {
			widget.ts[ch] = d - 600*(widget.minutes ? 60 : 1);
			for (var i = 0; i < 600; i++) {
				widget.data[ch][i] = 0;
			}
			xhr.open("GET", live+name+":meter"+params);
			xhr.send();
		} else {
			widget.data[ch].push(d[0]);
			widget.data[ch].shift();
			widget.ts[ch] += widget.minutes ? 60 : 1;
			refresh();
		}
	};
	xhr.onreadystatechange = function (ev) {
		if (xhr.readyState != 4) return;
		if (widget.xhr[ch] !== xhr) return;
		var d = JSON.parse(xhr.responseText), t = d.shift();
		var diff = (widget.ts[ch] - t) / (widget.minutes ? 60 : 1);
		for (var i = 0; i < d.length; i++) {
			var j = i-diff;
			if (j > 0 && j < widget.data[ch].length) {
				widget.data[ch][j] = d[i][0];
			}
		}
		if (!widget.on) {
			widget.on = true;
			widget.loading = false;
			clearInterval(widget.blink);
			widget.ledOn();
		}
		refresh();
	};

	function refresh() {
		if (!widget.on) return;
		if (widget.ts[0] !== null && widget.ts[0] === widget.ts[1]) {
			widget.draw();
		}
	}
}

Widget.prototype.ledOn = function () {
	var el = this.el.getElementsByClassName('led')[0];
	el.classList.remove('off');
	el.classList.add('on');
};

Widget.prototype.ledOff = function () {
	var el = this.el.getElementsByClassName('led')[0];
	el.classList.remove('on');
	el.classList.add('off');
};

Widget.prototype.toggleLed = function () {
	var el = this.el.getElementsByClassName('led')[0];
	el.classList.toggle('on');
	el.classList.toggle('off');
}

Widget.prototype.draw = function draw() {
	var canvas = this.el.getElementsByTagName('canvas')[0];
	var context = canvas.getContext('2d');
	var width = canvas.width;
	var height = canvas.height;

	var max = 0;
	for (var i = this.data[0].length-width; i < this.data[0].length; i++) {
		if (this.data[0][i] > max) {
			max = this.data[0][i];
		}
	}
	for (var i = this.data[1].length-width; i < this.data[1].length; i++) {
		if (this.data[1][i] > max) {
			max = this.data[1][i];
		}
	}

	var M = 10, N;
	if (max > 2) {
		for (;;) {
			if (max > M) {
				M *= 10;
			} else {
				N = Math.ceil(max/(M/10));
				scale = N*(M/10);
				if (N <= 2) {
					N = M/20;
				} else if (N <= 5) {
					N = M/10;
				} else {
					N = M/5;
				}
				break;
			}
		}
	} else {
		scale = 2;
		N = 1;
	}

	var D, U;
	if (N < 1e3) {
		D = 1;
		U = '';
	} else if (N < 1e6) {
		D = 1e3;
		U = 'k';
	} else if (N < 1e9) {
		D = 1e6;
		U = 'M';
	} else if (N < 1e12) {
		D = 1e9;
		U = 'G';
	} else {
		D = 1e12;
		U = 'T';
	}

	context.rect(0, 0, width, height);
	context.fillStyle = '#ffe';
	context.fill();

	context.beginPath();
	for (var i = width-1; i >= 0; i--) {
		var val, j = i + this.data[0].length - width;
		if (j >= 0) {
			val = height*(1-this.data[0][j]/scale);
		} else {
			val = height;
		}
		if (i == width-1) {
			context.lineTo(width, height);
			context.lineTo(width, val-0.5);
		}
		context.lineTo(i, val-0.5);
	}
	context.lineTo(0, height);
	var gradient = context.createLinearGradient(0, 0, 0, height);
	gradient.addColorStop(0, '#0b0');
	gradient.addColorStop(1, '#cc0');
	context.fillStyle = gradient;
	context.fill();

	context.beginPath();
	for (var i = width-1; i >= 0; i--) {
		var val, j = i + this.data[1].length - width;
		if (j >= 0) {
			val = height*(1-this.data[1][j]/scale);
		} else {
			val = height;
		}
		context.lineTo(i, val-0.5);
	}
	context.strokeStyle = '#00f';
	context.stroke();

	var localTs = this.ts[0] - 60*(new Date).getTimezoneOffset();
	localTs += (this.data[0].length - width)*(this.minutes ? 60 : 1);
	context.strokeStyle = '#000';
	context.fillStyle = '#444';
	context.textAlign = 'center';
	if (this.minutes) {
		var localMin = Math.floor(localTs / 60);
		for (var x = localMin % 60 ? 60-localMin%60 : 0; x < width; x += 60) {
			context.beginPath();
			context.moveTo(x+0.5, 0);
			context.lineTo(x+0.5, height);
			context.globalAlpha = 0.25;
			context.stroke();
			context.globalAlpha = 1.0;
			var h = ((localMin+x) / 60) % 24;
			if (h < 10) h = '0'+h;
			context.fillText(h+':00', x, height-2);
		}
	} else {
		for (var x = localTs % 60 ? 60-localTs%60 : 0; x < width; x += 60) {
			context.beginPath();
			context.moveTo(x+0.5, 0);
			context.lineTo(x+0.5, height);
			context.globalAlpha = 0.25;
			context.stroke();
			context.globalAlpha = 1.0;
			var h = Math.floor((localTs+x) / 3600) % 24;
			if (h < 10) h = '0'+h;
			var m = Math.floor((localTs+x) / 60) % 60;
			if (m < 10) m = '0'+m;
			context.fillText(h+':'+m, x, height-2);
		}
	}

	context.strokeStyle = '#000';
	context.fillStyle = '#444';
	context.textAlign = 'right';
	var L = context.measureText((scale-N)/D+U).width;
	for (var y = N; y < scale; y += N) {
		var Y = height*(1-y/scale);
		Y |= 0;
		Y -= 0.5;
		context.beginPath();
		context.moveTo(0, Y);
		context.lineTo(width, Y);
		context.globalAlpha = 0.25;
		context.stroke();
	
		context.globalAlpha = 1.0;
		context.fillText((y/D)+U, L+2, Y-2);
	}

	context.textAlign = 'left';
	context.fillStyle = '#080';
	context.fillText("Rx", 2, 10);
	context.textAlign = 'right';
	context.fillText(formatNumber(this.data[0][this.data[0].length-1]), 60, 10)

	context.textAlign = 'left';
	context.fillStyle = '#00f';
	context.fillText("Tx", 2, 22);
	context.textAlign = 'right';
	context.fillText(formatNumber(this.data[1][this.data[1].length-1]), 60, 22)

}

function formatNumber(N) {
	var D, U;
	if (N < 1e3) {
		D = 1;
		U = '';
	} else if (N < 1e6) {
		D = 1e3;
		U = 'k';
	} else if (N < 1e9) {
		D = 1e6;
		U = 'M';
	} else if (N < 1e12) {
		D = 1e9;
		U = 'G';
	} else {
		D = 1e12;
		U = 'T';
	}
	N /= D;
	if (N < 10) {
		D = 1000;
	} else if (N < 100) {
		D = 100;
	} else {
		D = 10;
	}
	N = (Math.floor(N*D)/D) + '';
	if (N.indexOf('.') == -1) {
		if (N.length < 4) {
			N += '.';
			while (N.length < 5) N += '0';
		}
	} else {
		while (N.length < 5) N += '0';
	}
	return N+U;
}
