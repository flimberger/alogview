# Tools
GOTOOL?=	go
INSTALL?=	install

# Locations
DESTDIR?=	/usr/local
BINDIR?=	/bin

PROG=	alogview

all: ${PROG}

${PROG}: *.go
	${GOTOOL} build -o ${PROG}

install: ${PROG}
	install -C ${PROG} ${DESTDIR}${BINDIR}/${PROG}

clean:
	rm -f ${PROG}

.PHONY: all install clean
