module.exports = function(grunt) {
	require('load-grunt-tasks')(grunt);

	var path = require('path');

	var lessCreateConfig = function (context, block) {
		var cfg = {files: []},
		outfile = path.join(context.outDir, block.dest),
		filesDef = {};

		filesDef.dest = outfile;
		filesDef.src = [];

		context.inFiles.forEach(function (inFile) {
			filesDef.src.push(path.join(context.inDir, inFile));
		});

		cfg.files.push(filesDef);
		context.outFiles = [block.dest];
		return cfg;
	};

	grunt.initConfig({
		pkg: grunt.file.readJSON('package.json'),
		paths: {
			in: {
				assets: 'public',
				templates: 'templates',
				brand: 'brand-overlay',
			},
			out: {
				root: 'build',
				assets: '<%= paths.out.root %>/public',
				templates: '<%= paths.out.root %>/templates',
			},
		},
		clean: {
			build: ['build', '.tmp'],
		},
		useminPrepare: {
			html: 'templates/_lib.tmpl',
			options: {
				root: '<%= paths.in.assets %>',
				dest: '<%= paths.out.assets %>',
				flow: {
					html : {
						steps: {
							'less': ['concat', {
								name: 'less',
								createConfig: lessCreateConfig
							}, 'cssmin'],
							'css': ['concat', 'cssmin'],
							'js': ['concat', 'uglify'],
						},
					},
				},
			},
		},
		usemin: {
			html: '<%= paths.out.templates %>/_lib.tmpl',
			options: {
				assetsDirs: '<%= paths.out.assets %>',
				blockReplacements: {
					less: function (block) {
						return '<link rel="stylesheet" href="' + block.dest + '" media="all">';
					}
				},
			},
		},
		filerev: {
			options: {
				algorithm: 'sha1',
				length: 8,
			},
			assets: {
				src: [
					'<%= paths.out.assets %>/js/*.js',
					'<%= paths.out.assets %>/css/*.css',
				],
			},
		},
		copy: {
			main: {
				files: [
				{
					expand: true,
					cwd: '<%= paths.in.assets %>/',
					src: [
						'**/*.png',
						'**/*.gif',
						'**/*.sh',
						'**/*.ico',
						'**/*.html',
						'**/*.txt',
						'fonts/**',
					],
					dest: '<%= paths.out.assets %>/',
				},
				{
					expand: true,
					cwd: '<%= paths.in.templates %>/',
					src: [
						'**/*.tmpl',
					],
					dest: '<%= paths.out.templates %>/',
				},
				/* Brand overlay stuff */
				{
					expand: true,
					cwd: '<%= paths.in.brand %>/public/',
					src: [
						'**/*',
					],
					dest: '<%= paths.out.assets %>/',
				},
				{
					expand: true,
					cwd: '<%= paths.in.brand %>/templates/',
					src: [
						'**/*.tmpl',
					],
					dest: '<%= paths.out.templates %>/',
				}
				],
			},
		},
	});

	grunt.registerTask('default', [
		'clean',
		'useminPrepare',
		'concat:generated',
		'less:generated',
		'uglify:generated',
		'cssmin:generated',
		'copy',
		'filerev',
		'usemin',
	]);
};
