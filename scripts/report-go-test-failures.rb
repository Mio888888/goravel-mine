#!/usr/bin/env ruby
# frozen_string_literal: true

require "json"

events = []
File.foreach(ARGV.fetch(0)) do |line|
  events << JSON.parse(line)
rescue JSON::ParserError
  next
end

failures = events.select { |event| event["Action"] == "fail" && event["Test"] }
if failures.empty?
  package_failure = events.reverse.find { |event| event["Action"] == "fail" }
  warn(package_failure ? "Go test package failed without a test-level failure event" : "Go tests passed")
  exit
end

failures.map { |event| event["Test"] }.uniq.each do |test_name|
  warn "\n--- #{test_name} ---"
  output = events.select { |event| event["Test"] == test_name }
                 .map { |event| event["Output"] }
                 .compact
                 .last(120)
  output.each { |line| warn line }
end
