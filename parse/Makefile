all : parser.go

# goyacc grows its parser stack without a limit. Make the generated code
# grow it with yyGrowStack (parser.go.y), which stops at maxParseStack.
parser.go : parser.go.y
	goyacc -o $@ parser.go.y; [ -f y.output ] && ( rm -f y.output )
	sed -i.bak 's/nyys := make(\[\]yySymType, len(yyS)\*2)/nyys := yyGrowStack(yylex, yyS)/' $@ && rm -f $@.bak
	@grep -q 'yyGrowStack(yylex, yyS)' $@ || (echo "parser.go: stack growth not patched" && false)
	gofmt -w $@

clean:
	rm -f parser.go
