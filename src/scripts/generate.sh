# get first argument
if [ -z "$1" ]; then
    echo "Please provide a year e.g. 'generate.sh 2023'"
    exit 1
fi


rm README.md subreddits.txt wordcloud_$1.png;

go run generate-stats.go $1;

wordcloud_cli \
--text subreddits.txt \
--imagefile wordcloud_$1.png \
--color red \
--background white \
--height 2000 \
--width 2000 \
--margin 10 \
--mask ./circle.png
