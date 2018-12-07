GO=go

DESTDIR ?= ""
SYSCONFDIR ?= /etc
INSTALLDIR = $(DESTDIR)$(SYSCONFDIR)/kubernetes/node-feature-discovery/source.d/

.PHONY: all install

binaries = cpu memory

all: $(binaries)

$(binaries): %: %.go
	$(GO) build $<

install: $(binaries)
	install -d $(INSTALLDIR)
	install -t $(INSTALLDIR) $(binaries)

