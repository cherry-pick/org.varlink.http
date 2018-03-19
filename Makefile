all:
	go build github.com/varlink/org.varlink.http
.PHONY: all

clean:
	rm -f org.varlink.http
.PHONY: clean
