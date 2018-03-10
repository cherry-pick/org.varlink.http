all:
	go build -o org.varlink.http
.PHONY: all

clean:
	rm -f org.varlink.http
.PHONY: clean
