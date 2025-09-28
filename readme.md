# (S)pring (Lo)g (P)arser
A simple parser built to serialize and pretty-print SpringBoot logs piped to Stdin.

## Usage 

```
cat example/multi_line_log.txt | slop -level <LEVEL> -grep <SEACH>
```

## Configuration
SLoP creates a dot directory in your $HOME named `.slop`. The next few commits will implement a way to configure your Regexp group filters.

